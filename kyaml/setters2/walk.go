// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package setters2

import (
	"sigs.k8s.io/kustomize/kyaml/fieldmeta"
	"sigs.k8s.io/kustomize/kyaml/openapi"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// visitor is implemented by structs which need to walk the configuration.
// visitor is provided to accept to walk configuration
type visitor interface {
	// visitScalar is called for each scalar field value on a resource
	// node is the scalar field value
	// path is the path to the field; path elements are separated by '.'
	// oa is the OpenAPI schema for the field
	visitScalar(node *yaml.RNode, path string, oa *openapi.ResourceSchema) error

	// visitSequence is called for each sequence field value on a resource
	// node is the sequence field value
	// path is the path to the field
	// oa is the OpenAPI schema for the field
	visitSequence(node *yaml.RNode, path string, oa *openapi.ResourceSchema) error

	// visitMapping is called for each Mapping field value on a resource
	// node is the mapping field value
	// path is the path to the field
	// oa is the OpenAPI schema for the field
	visitMapping(node *yaml.RNode, path string, oa *openapi.ResourceSchema) error
}

// accept invokes the appropriate function on v for each field in object
func accept(v visitor, object *yaml.RNode) error {
	// get the OpenAPI for the type if it exists
	oa := getSchema(object, nil, "")
	return acceptImpl(v, object, "", oa)
}

// acceptImpl implements accept using recursion
func acceptImpl(v visitor, object *yaml.RNode, p string, oa *openapi.ResourceSchema) error {
	switch object.YNode().Kind {
	case yaml.DocumentNode:
		// Traverse the child of the document
		return accept(v, yaml.NewRNode(object.YNode()))
	case yaml.MappingNode:
		if err := v.visitMapping(object, p, oa); err != nil {
			return err
		}
		return object.VisitFields(func(node *yaml.MapNode) error {
			// get the schema for the field and propagate it
			oa = getSchema(node.Key, oa, node.Key.YNode().Value)
			// Traverse each field value
			return acceptImpl(v, node.Value, p+"."+node.Key.YNode().Value, oa)
		})
	case yaml.SequenceNode:
		// get the schema for the sequence node, use the schema provided if not present
		// on the field
		if err := v.visitSequence(object, p, oa); err != nil {
			return err
		}
		// get the schema for the elements
		oa = getSchema(object, oa, "")
		return object.VisitElements(func(node *yaml.RNode) error {
			// Traverse each list element
			return acceptImpl(v, node, p, oa)
		})
	case yaml.ScalarNode:
		// Visit the scalar field
		oa = getSchema(object, oa, "")
		return v.visitScalar(object, p, oa)
	}
	return nil
}

// getSchema returns OpenAPI schema for an RNode or field of the
// RNode.  It will overriding the provide schema with field specific values
// if they are found
// r is the Node to get the Schema for
// s is the provided schema for the field if known
// field is the name of the field
func getSchema(r *yaml.RNode, s *openapi.ResourceSchema, field string) *openapi.ResourceSchema {
	// get the override schema if it exists on the field
	fm := fieldmeta.FieldMeta{}
	if err := fm.Read(r); err == nil && !fm.IsEmpty() {
		// per-field schema, this is fine
		if fm.Schema.Ref.String() != "" {
			// resolve the reference
			s, err := openapi.Resolve(&fm.Schema.Ref)
			if err == nil && s != nil {
				fm.Schema = *s
			}
		}
		return &openapi.ResourceSchema{Schema: &fm.Schema}
	}

	// get the schema for a field of the node if the field is provided
	if s != nil && field != "" {
		return s.Field(field)
	}

	// get the schema for the elements if this is a list
	if s != nil && r.YNode().Kind == yaml.SequenceNode {
		return s.Elements()
	}

	// use the provided schema if present
	if s != nil {
		return s
	}

	if yaml.IsEmpty(r) {
		return nil
	}

	// lookup the schema for the type
	m, _ := r.GetMeta()
	if m.Kind == "" || m.APIVersion == "" {
		return nil
	}
	return openapi.SchemaForResourceType(yaml.TypeMeta{Kind: m.Kind, APIVersion: m.APIVersion})
}
