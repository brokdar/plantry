// Package spectest holds the spec-derived property tests of the assistant
// (loop/spec/behaviour-v1.md). Every test file carries the build tag
// `spectest`, so the normal gate stays green while product code catches up:
//
//	go test -tags spectest ./internal/spectest/...
//
// Each test names the spec rule it checks. The tests are owned by the eval
// owner; product workers must not edit them.
package spectest
