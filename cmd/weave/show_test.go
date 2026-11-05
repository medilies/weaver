package main

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/medilies/weaver/internal/session/app"
	"github.com/medilies/weaver/internal/session/domain"
)

// storedDocuments is a fake app.ProjectDocuments: project name → config.
type storedDocuments map[string]string

func (d storedDocuments) Document(name domain.ProjectName) (app.ConfigDocument, error) {
	content, ok := d[name.String()]
	if !ok {
		return app.ConfigDocument{}, fmt.Errorf("%w: %s", domain.ErrProjectNotFound, name)
	}
	return app.ConfigDocument{File: name.String() + ".yml", Content: []byte(content)}, nil
}

func (d storedDocuments) ReplaceDocument(domain.ProjectName, []byte) error {
	return nil
}

func TestShowCommand(t *testing.T) {
	const config = "# yaml-language-server: $schema=./weaver.schema.json\nroot: ~/dev/shop # kept as written\n"
	root := newRootCommand(application{documents: storedDocuments{"shop": config}})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"show", "shop"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := out.String(); got != config {
		t.Fatalf("got %q, want the config file as stored", got)
	}
}
