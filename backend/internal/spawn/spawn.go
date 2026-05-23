// Package spawn is the high-level orchestrator that turns a Genome into a
// concrete k8s Pod, launches it, awaits completion, and parses the final
// result JSON line from stdout.
package spawn
