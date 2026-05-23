// Package k8s — namespace lifecycle helpers.
//
// EnsureNamespace lets the control plane bring up the openzerg namespace
// idempotently on a fresh DigitalOcean cluster. It mirrors the labels in
// backend/deploy/namespace.yaml so manifest-applied and code-applied
// namespaces are indistinguishable.
package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// NamespaceLabels are the labels EnsureNamespace stamps onto namespaces it
// creates. They match backend/deploy/namespace.yaml. Existing namespaces are
// NOT relabeled — we only set labels on the create path so we don't fight
// with whatever the human or another tool already wrote.
var NamespaceLabels = map[string]string{
	"app.kubernetes.io/name":       "openzerg",
	"app.kubernetes.io/part-of":    "openzerg",
	"app.kubernetes.io/managed-by": "openzerg-control-plane",
}

// EnsureNamespace creates the named namespace if it does not already exist.
// It is idempotent: an AlreadyExists error from the apiserver is treated as
// success (Created=false). On a successful create, Created=true is returned
// so callers can log "created namespace" only when something actually
// changed.
//
// EnsureNamespace deliberately does not patch labels or annotations on
// pre-existing namespaces; the openzerg control plane should not silently
// rewrite a namespace it didn't create.
func EnsureNamespace(ctx context.Context, cs kubernetes.Interface, name string) (created bool, err error) {
	if cs == nil {
		return false, fmt.Errorf("k8s.EnsureNamespace: nil clientset")
	}
	if name == "" {
		return false, fmt.Errorf("k8s.EnsureNamespace: empty name")
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: NamespaceLabels,
		},
	}
	_, err = cs.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err == nil {
		return true, nil
	}
	if apierrors.IsAlreadyExists(err) {
		return false, nil
	}
	return false, fmt.Errorf("k8s.EnsureNamespace: create %q: %w", name, err)
}
