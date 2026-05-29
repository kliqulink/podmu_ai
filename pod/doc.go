// Package pod implements the Pod abstraction from docs/specs/pod-spec.md:
// the canonical Pod manifest (pod.yaml), the on-disk Pod Bundle format, and
// loading/validation against the V1 spec.
//
// A Pod separates two planes (pod-spec §2):
//
//   - Definition plane — authored, versioned: the manifest plus identity,
//     agents, workflows, tools, branding, knowledge, deployments, permissions.
//   - State plane — accumulated, snapshotted: memory, analytics, business
//     state, the event log (under state/).
//
// This package covers the Definition root: parsing and validating the manifest
// and the bundle layout, and the runtime compatibility handshake (pod-spec
// §9.2). It deliberately does not execute anything — a Bundle is inert
// serialized state (pod-spec §10).
package pod
