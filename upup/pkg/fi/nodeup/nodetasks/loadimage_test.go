/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodetasks

import (
	"bytes"
	"compress/gzip"
	"io"
	"reflect"
	"testing"

	"k8s.io/kops/upup/pkg/fi"
)

func TestLoadImageTask_Deps(t *testing.T) {
	l := &LoadImageTask{}

	tasks := make(map[string]fi.NodeupTask)
	tasks["LoadImageTask1"] = &LoadImageTask{}
	tasks["FileTask1"] = &File{}
	tasks["ServiceDocker"] = &Service{Name: "docker.service"}
	tasks["Service2"] = &Service{Name: "two.service"}

	deps := l.GetDependencies(tasks)
	expected := []fi.NodeupTask{tasks["ServiceDocker"]}
	if !reflect.DeepEqual(expected, deps) {
		t.Fatalf("unexpected deps.  expected=%v, actual=%v", expected, deps)
	}
}

func TestMaybeGzipReaderRejectsCorruptGzip(t *testing.T) {
	// Bytes start with the gzip magic but don't form a valid header.
	corrupt := bytes.NewReader([]byte{0x1f, 0x8b, 0x08, 0x00})
	if _, err := maybeGzipReader(corrupt); err == nil {
		t.Fatalf("maybeGzipReader() expected error for truncated gzip header")
	}
}

func TestMaybeGzipReaderPassesThroughUncompressed(t *testing.T) {
	body := []byte("plain tar bytes")
	r, err := maybeGzipReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("maybeGzipReader() error = %v", err)
	}
	defer r.Close()
	got := make([]byte, len(body))
	if _, err := r.Read(got); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("maybeGzipReader() body = %q, expected %q", got, body)
	}

	// Sanity: round-trip through gzip works too.
	var compressed bytes.Buffer
	gw := gzip.NewWriter(&compressed)
	_, _ = gw.Write(body)
	_ = gw.Close()
	r2, err := maybeGzipReader(bytes.NewReader(compressed.Bytes()))
	if err != nil {
		t.Fatalf("maybeGzipReader(gzip) error = %v", err)
	}
	defer r2.Close()
	got2, err := io.ReadAll(r2)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got2, body) {
		t.Fatalf("maybeGzipReader(gzip) body = %q, expected %q", got2, body)
	}
}
