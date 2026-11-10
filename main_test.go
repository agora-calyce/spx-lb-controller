package main

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const testProjectID = "3f2b8c1e-5d4a-4f6b-9a7c-1e2d3c4b5a69"

func nodeWithAnnotations(annotations map[string]string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1", Annotations: annotations}}
}

func TestDiscoverProjectID(t *testing.T) {
	tests := []struct {
		name    string
		objects []runtime.Object
		want    string
		wantErr bool
	}{
		{
			name:    "prefixed annotation",
			objects: []runtime.Object{nodeWithAnnotations(map[string]string{projectIDAnnotation: "spx-" + testProjectID})},
			want:    testProjectID,
		},
		{
			name:    "annotation without prefix",
			objects: []runtime.Object{nodeWithAnnotations(map[string]string{projectIDAnnotation: testProjectID})},
			want:    testProjectID,
		},
		{
			name:    "no nodes",
			wantErr: true,
		},
		{
			name:    "missing annotation",
			objects: []runtime.Object{nodeWithAnnotations(nil)},
			wantErr: true,
		},
		{
			name:    "empty annotation",
			objects: []runtime.Object{nodeWithAnnotations(map[string]string{projectIDAnnotation: ""})},
			wantErr: true,
		},
		{
			name:    "invalid UUID",
			objects: []runtime.Object{nodeWithAnnotations(map[string]string{projectIDAnnotation: "spx-not-a-uuid"})},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tt.objects...)
			got, err := discoverProjectID(context.Background(), clientset)
			if (err != nil) != tt.wantErr {
				t.Fatalf("discoverProjectID() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("discoverProjectID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDiscoverProjectIDListError(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden")
	})

	if _, err := discoverProjectID(context.Background(), clientset); err == nil {
		t.Fatal("discoverProjectID() expected an error when listing nodes fails")
	}
}
