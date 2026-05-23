// Package spawn — single-pod orchestrator.
//
// RunPod is the smallest end-to-end glue that the M2 run-loop will call once
// per pod: it creates the pod, streams stdout, waits for the pod to reach a
// terminal phase, parses the final JSON line as the attacker result, and then
// (always, via defer) deletes the pod. Multi-pod fan-out lives one layer up
// in the cmd/openzerg run loop.
//
// The function intentionally does no fitness scoring; that is evolve's job.
// It only contracts to either return a parsed map[string]any (the result
// line) plus the raw line bytes, or an error describing which stage failed.
package spawn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/TheApexWu/openzerg/backend/internal/evolve"
	"github.com/TheApexWu/openzerg/backend/internal/k8s"
)

// PodResult is the outcome of a single pod run from spawn's perspective.
// Fitness scoring and evidence interpretation happen in evolve.
type PodResult struct {
	// Pod is the final pod object after the terminal Get. Its Status
	// fields (Phase, ContainerStatuses) describe how the container exited.
	Pod *corev1.Pod
	// RawLine is the bytes of the last JSON line emitted on stdout. Empty
	// when no JSON line was found.
	RawLine []byte
	// Result is the decoded final JSON object. Nil when no JSON line was
	// found; spawn's caller treats that as fitness=0.0.
	Result map[string]any
	// ParseError is set when stdout streamed cleanly but no JSON object
	// could be parsed from it. The pod is still considered "completed";
	// the caller decides how to score the absence of a result.
	ParseError error
}

// RunPod orchestrates one pod lifecycle: create → stream logs → wait for
// terminal phase → parse → delete. The pod is always deleted, including on
// any error along the way, so callers do not need their own defer.
//
// Context cancellation aborts the wait/stream and triggers cleanup. The
// returned error reflects the first stage to fail; cleanup errors are
// silently best-effort because there is no useful recovery path for a
// stuck-delete during a hackathon demo.
func RunPod(ctx context.Context, cs kubernetes.Interface, pod *corev1.Pod) (*PodResult, error) {
	if cs == nil {
		return nil, fmt.Errorf("spawn.RunPod: nil clientset")
	}
	if pod == nil {
		return nil, fmt.Errorf("spawn.RunPod: nil pod")
	}

	created, err := k8s.CreatePod(ctx, cs, pod)
	if err != nil {
		return nil, fmt.Errorf("spawn.RunPod: create: %w", err)
	}
	// Best-effort delete on every exit path. Use a fresh context so a
	// caller-cancelled ctx does not prevent cleanup.
	defer func() {
		_ = k8s.DeletePod(context.Background(), cs, created.Namespace, created.Name)
	}()

	// Wait for terminal phase before reading logs. If we open a follow=true
	// stream too early (before the container has started) the API server
	// returns "container ... is waiting to start" and we get nothing. The
	// pods we run print their result and exit in a couple of seconds; we
	// can simply wait, then read the now-complete log buffer in one shot.
	final, waitErr := k8s.WaitForCompletion(ctx, cs, created.Namespace, created.Name)
	if waitErr != nil {
		return nil, fmt.Errorf("spawn.RunPod: wait: %w", waitErr)
	}

	// Logs are guaranteed available once the pod is terminal. Follow=false
	// would be more honest, but we reuse StreamLogs to keep one code path;
	// against a finished pod, follow=true returns immediately at EOF.
	var raw []byte
	stream, streamErr := k8s.StreamLogs(ctx, cs, created.Namespace, created.Name, "")
	if streamErr == nil {
		raw, _ = io.ReadAll(stream)
		_ = stream.Close()
	}

	res := &PodResult{Pod: final}
	if len(raw) > 0 {
		line, obj, perr := evolve.ParseLastJSONLineString(string(raw))
		if perr == nil {
			res.RawLine = line
			res.Result = obj
		} else if !errors.Is(perr, evolve.ErrNoJSONLine) {
			res.ParseError = perr
		} else {
			res.ParseError = perr
		}
	} else {
		res.ParseError = evolve.ErrNoJSONLine
	}
	return res, nil
}

// PodOutcome pairs a pod result with its run-level error (if any), preserving
// the input index so callers can correlate fan-out results with their inputs.
// A non-nil Err means RunPod itself failed for that pod (create / wait); the
// Result is nil in that case. A parsed-but-empty result (no JSON line on
// stdout) is signalled via Result.ParseError, not Err.
type PodOutcome struct {
	// Index is the position of this pod in the input slice passed to
	// RunPods. Useful when the caller wants to align outcomes back to
	// their genome metadata.
	Index int
	// Result is the spawn-level outcome for this pod. Nil when Err != nil.
	Result *PodResult
	// Err is the first stage failure from RunPod (create / wait). Nil on
	// a clean run, even if no JSON result line was produced.
	Err error
}

// RunPods fans out RunPod across the input pods, running them concurrently.
// All outcomes are collected (one per input pod) and returned in input order.
// A failure of any individual pod does NOT abort siblings; the swarm only
// converges when every pod has either completed or been cleaned up. Context
// cancellation propagates into each RunPod call.
//
// This is the M2 fan-out primitive that the cmd/openzerg run loop will call
// once per generation. It deliberately does not enforce a parallelism cap:
// the population sizes contemplated (≤15) are small enough that the kube
// apiserver, not the control plane, becomes the bottleneck.
func RunPods(ctx context.Context, cs kubernetes.Interface, pods []*corev1.Pod) ([]PodOutcome, error) {
	if cs == nil {
		return nil, fmt.Errorf("spawn.RunPods: nil clientset")
	}
	outcomes := make([]PodOutcome, len(pods))
	var wg sync.WaitGroup
	for i, p := range pods {
		i, p := i, p
		outcomes[i].Index = i
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := RunPod(ctx, cs, p)
			outcomes[i].Result = res
			outcomes[i].Err = err
		}()
	}
	wg.Wait()
	return outcomes, nil
}
