package warden

import (
	"reflect"
	"testing"
)

func TestGroupIssues(t *testing.T) {
	got := groupIssues([]string{
		"no backup replicas configured",
		"unknown running process: acpid",
		"service config has no reload mapping: /etc/login.defs",
		"unknown running process: snapd",
		"unknown running process: fwupd",
	})
	want := []string{
		"no backup replicas configured",
		"unknown running process (3): acpid, snapd, fwupd",
		"service config has no reload mapping: /etc/login.defs",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}
