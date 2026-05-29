// Command podctl loads and inspects Pod Bundles — the first runnable surface
// over the pod package. It turns the Pod spec into something you can point at a
// directory and get an answer from.
//
// Usage:
//
//	podctl validate <bundle-dir>   validate a bundle (manifest + refs + compatibility)
//	podctl info     <bundle-dir>   print a summary of a bundle
//	podctl id                      generate a fresh Pod id
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/kliqulink/podmu_ai/pod"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "validate":
		os.Exit(cmdValidate(os.Args[2:]))
	case "info":
		os.Exit(cmdInfo(os.Args[2:]))
	case "id":
		os.Exit(cmdID())
	case "-h", "--help", "help":
		usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "podctl: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `podctl — inspect Podmu Pod Bundles

usage:
  podctl validate <bundle-dir>   validate a bundle (manifest + refs + runtime compatibility)
  podctl info     <bundle-dir>   print a summary of a bundle
  podctl id                      generate a fresh Pod id
`)
}

func cmdValidate(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: podctl validate <bundle-dir>")
		return 2
	}
	b, err := pod.Load(args[0])
	if err != nil {
		if ve, ok := errors.AsType[pod.ValidationErrors](err); ok {
			fmt.Fprintf(os.Stderr, "INVALID: %s\n\n%d problem(s):\n", args[0], len(ve))
			for _, e := range ve {
				fmt.Fprintf(os.Stderr, "  - %s\n", e.Error())
			}
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		}
		return 1
	}
	if err := pod.CheckCompatibility(b.Manifest, pod.CurrentRuntimeVersion); err != nil {
		fmt.Fprintf(os.Stderr, "INCOMPATIBLE: %v\n", err)
		return 1
	}
	fmt.Printf("OK: %s (%s, runtime %s compatible)\n",
		b.Manifest.Metadata.Slug, b.Materialization(), pod.CurrentRuntimeVersion)
	return 0
}

func cmdInfo(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: podctl info <bundle-dir>")
		return 2
	}
	b, err := pod.Load(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		return 1
	}
	m := b.Manifest
	compat := "yes"
	if err := pod.CheckCompatibility(m, pod.CurrentRuntimeVersion); err != nil {
		compat = "NO — " + err.Error()
	}
	fmt.Printf("id:             %s\n", m.Metadata.ID)
	fmt.Printf("slug:           %s\n", m.Metadata.Slug)
	fmt.Printf("owner:          %s\n", m.Metadata.OwnerID)
	fmt.Printf("brand:          %s\n", m.Spec.Identity.Brand)
	fmt.Printf("pod_version:    %d\n", m.Spec.PodVersion)
	fmt.Printf("apiVersion:     %s\n", m.APIVersion)
	fmt.Printf("min_version:    %s  (compatible: %s)\n", m.Spec.Runtime.MinVersion, compat)
	fmt.Printf("materialization:%s\n", b.Materialization())
	fmt.Printf("agents:         %d\n", len(m.Spec.Agents))
	fmt.Printf("workflows:      %d\n", len(m.Spec.Workflows))
	fmt.Printf("tools:          %d\n", len(m.Spec.Tools))
	fmt.Printf("deployments:    %d\n", len(m.Spec.Deployments))
	fmt.Printf("memory stores:  %v\n", m.Spec.Memory.Stores)
	return 0
}

func cmdID() int {
	id, err := pod.NewPodID()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		return 1
	}
	fmt.Println(id)
	return 0
}
