package main

// Registers the oidc auth provider, so a kubeconfig that logs in through it
// loads like it does for kubectl.
import _ "k8s.io/client-go/plugin/pkg/client/auth"
