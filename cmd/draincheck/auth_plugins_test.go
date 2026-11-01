package main

import (
	"os"
	"path/filepath"
	"testing"
)

const oidcKubeconfig = `apiVersion: v1
kind: Config
current-context: c
clusters:
- name: c
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: c
  context: {cluster: c, user: u}
users:
- name: u
  user:
    auth-provider:
      name: oidc
      config:
        idp-issuer-url: https://issuer.example.invalid
        client-id: draincheck
`

// A kubeconfig that logs in through the oidc auth provider must load. client-go
// only knows the provider after the auth plugins package is imported.
func TestNewClientsetAcceptsOIDCAuthProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(oidcKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newClientset(path, "", nil); err != nil {
		t.Fatalf("newClientset with oidc auth-provider: %v", err)
	}
}
