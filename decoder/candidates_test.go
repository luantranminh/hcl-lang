// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package decoder

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl-lang/lang"
	"github.com/hashicorp/hcl-lang/schema"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/json"
	"github.com/zclconf/go-cty-debug/ctydebug"
	"github.com/zclconf/go-cty/cty"
)

func TestDecoder_CompletionAtPos_noSchema(t *testing.T) {
	ctx := context.Background()
	f, pDiags := hclsyntax.ParseConfig(testConfig, "test.tf", hcl.InitialPos)
	if len(pDiags) > 0 {
		t.Fatal(pDiags)
	}

	d := testPathDecoder(t, &PathContext{
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	_, err := d.CompletionAtPos(ctx, "test.tf", hcl.InitialPos)
	noSchemaErr := &NoSchemaError{}
	if !errors.As(err, &noSchemaErr) {
		t.Fatal("expected NoSchemaError for no schema")
	}
}

func TestDecoder_CompletionAtPos_emptyBody(t *testing.T) {
	ctx := context.Background()
	f := &hcl.File{
		Body: hcl.EmptyBody(),
	}

	d := testPathDecoder(t, &PathContext{
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	_, err := d.CompletionAtPos(ctx, "test.tf", hcl.InitialPos)
	unknownFormatErr := &UnknownFileFormatError{}
	if !errors.As(err, &unknownFormatErr) {
		t.Fatal("expected UnknownFileFormatError for empty body")
	}
}

func TestDecoder_CompletionAtPos_json(t *testing.T) {
	ctx := context.Background()
	f, pDiags := json.Parse([]byte(`{
	"customblock": {
		"label1": {}
	}
}`), "test.tf.json")
	if len(pDiags) > 0 {
		t.Fatal(pDiags)
	}

	d := testPathDecoder(t, &PathContext{
		Files: map[string]*hcl.File{
			"test.tf.json": f,
		},
	})

	_, err := d.CompletionAtPos(ctx, "test.tf.json", hcl.InitialPos)
	unknownFormatErr := &UnknownFileFormatError{}
	if !errors.As(err, &unknownFormatErr) {
		t.Fatal("expected UnknownFileFormatError for JSON body")
	}
}

func TestDecoder_CompletionAtPos_unknownBlock(t *testing.T) {
	ctx := context.Background()
	resourceLabelSchema := []*schema.LabelSchema{
		{Name: "type"},
		{Name: "name"},
	}
	resourceSchema := &schema.BlockSchema{
		Labels: resourceLabelSchema,
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"count": {Constraint: schema.LiteralType{Type: cty.Number}},
			},
		},
	}
	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"resource": resourceSchema,
		},
	}

	f, pDiags := hclsyntax.ParseConfig([]byte(`customblock "label1" {

}
`), "test.tf", hcl.InitialPos)
	if len(pDiags) > 0 {
		t.Fatal(pDiags)
	}

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	_, err := d.CompletionAtPos(ctx, "test.tf", hcl.Pos{
		Line:   2,
		Column: 1,
		Byte:   23,
	})
	if err == nil {
		t.Fatal("expected error for unknown block")
	}
	if !strings.Contains(err.Error(), "unknown block type") {
		t.Fatalf("unexpected error message: %q", err.Error())
	}
}

func TestDecoder_CompletionAtPos_nilBodySchema(t *testing.T) {
	ctx := context.Background()
	testCases := []struct {
		name               string
		rootSchema         *schema.BodySchema
		config             string
		pos                hcl.Pos
		expectedCandidates lang.Candidates
	}{
		{
			"nil static body",
			&schema.BodySchema{
				Blocks: map[string]*schema.BlockSchema{
					"resource": {
						Labels: []*schema.LabelSchema{
							{Name: "type"},
							{Name: "name"},
						},
						Body: nil,
					},
				},
			},
			`resource "label1" {
  count = 1

}
`,
			hcl.Pos{
				Line:   3,
				Column: 1,
				Byte:   32,
			},
			lang.ZeroCandidates(),
		},
		{
			"nil static body with dependent body",
			&schema.BodySchema{
				Blocks: map[string]*schema.BlockSchema{
					"resource": {
						Labels: []*schema.LabelSchema{
							{Name: "type", IsDepKey: true, Completable: true},
							{Name: "name"},
						},
						Body: nil,
						DependentBody: map[schema.SchemaKey]*schema.BodySchema{
							schema.NewSchemaKey(schema.DependencyKeys{
								Labels: []schema.LabelDependent{
									{Index: 0, Value: "label1"},
								},
							}): {
								Attributes: map[string]*schema.AttributeSchema{
									"one":   {Constraint: schema.LiteralType{Type: cty.String}},
									"two":   {Constraint: schema.LiteralType{Type: cty.Number}},
									"three": {Constraint: schema.LiteralType{Type: cty.Bool}},
								},
							},
						},
					},
				},
			},
			`resource "label1" {
  count = 1

}
`,
			hcl.Pos{
				Line:   3,
				Column: 1,
				Byte:   32,
			},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "one",
					Detail: "string",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start: hcl.Pos{
								Line:   3,
								Column: 1,
								Byte:   32,
							},
							End: hcl.Pos{
								Line:   3,
								Column: 1,
								Byte:   32,
							},
						},
						NewText: "one",
						Snippet: `one = "${1:value}"`,
					},
					Kind: lang.AttributeCandidateKind,
				},
				{
					Label:  "three",
					Detail: "bool",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start: hcl.Pos{
								Line:   3,
								Column: 1,
								Byte:   32,
							},
							End: hcl.Pos{
								Line:   3,
								Column: 1,
								Byte:   32,
							},
						},
						NewText: "three",
						Snippet: "three = ${1:false}",
					},
					Kind: lang.AttributeCandidateKind,
				},
				{
					Label:  "two",
					Detail: "number",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start: hcl.Pos{
								Line:   3,
								Column: 1,
								Byte:   32,
							},
							End: hcl.Pos{
								Line:   3,
								Column: 1,
								Byte:   32,
							},
						},
						NewText: "two",
						Snippet: "two = ${1:0}",
					},
					Kind: lang.AttributeCandidateKind,
				},
			}),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			f, pDiags := hclsyntax.ParseConfig([]byte(tc.config), "test.tf", hcl.InitialPos)
			if len(pDiags) > 0 {
				t.Fatal(pDiags)
			}

			d := testPathDecoder(t, &PathContext{
				Schema: tc.rootSchema,
				Files: map[string]*hcl.File{
					"test.tf": f,
				},
			})

			candidates, err := d.CompletionAtPos(ctx, "test.tf", tc.pos)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(tc.expectedCandidates, candidates); diff != "" {
				t.Fatalf("unexpected candidates: %s", diff)
			}
		})
	}
}

func TestDecoder_CompletionAtPos_prefixNearEOF(t *testing.T) {
	ctx := context.Background()
	resourceLabelSchema := []*schema.LabelSchema{
		{Name: "type"},
		{Name: "name"},
	}
	resourceSchema := &schema.BlockSchema{
		Labels: resourceLabelSchema,
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"count": {Constraint: schema.LiteralType{Type: cty.Number}},
			},
		},
	}
	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"resource": resourceSchema,
		},
	}

	f, _ := hclsyntax.ParseConfig([]byte(`res`), "test.tf", hcl.InitialPos)

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	candidates, err := d.CompletionAtPos(ctx, "test.tf", hcl.Pos{
		Line:   1,
		Column: 4,
		Byte:   3,
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedCandidates := lang.CompleteCandidates([]lang.Candidate{
		{
			Label:  "resource",
			Detail: "Block",
			TextEdit: lang.TextEdit{
				Range: hcl.Range{
					Filename: "test.tf",
					Start: hcl.Pos{
						Line:   1,
						Column: 1,
						Byte:   0,
					},
					End: hcl.Pos{
						Line:   1,
						Column: 4,
						Byte:   3,
					},
				},
				NewText: "resource",
				Snippet: "resource \"${1:type}\" \"${2:name}\" {\n  ${3}\n}",
			},
			Kind: lang.BlockCandidateKind,
		},
	})
	if diff := cmp.Diff(expectedCandidates, candidates, ctydebug.CmpOptions); diff != "" {
		t.Fatalf("candidates mismatch: %s", diff)
	}
}

func TestDecoder_CompletionAtPos_invalidBlockPositions(t *testing.T) {
	ctx := context.Background()
	resourceLabelSchema := []*schema.LabelSchema{
		{Name: "type"},
	}
	blockSchema := &schema.BlockSchema{
		Labels: resourceLabelSchema,
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"num_attr": {Constraint: schema.LiteralType{Type: cty.Number}},
			},
		},
	}
	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"myblock": blockSchema,
		},
	}
	testConfig := []byte(`myblock "foo" {

}
`)
	testCases := []struct {
		name string
		pos  hcl.Pos
	}{
		{
			"whitespace in header",
			hcl.Pos{
				Line:   1,
				Column: 14,
				Byte:   13,
			},
		},
		{
			"opening brace",
			hcl.Pos{
				Line:   1,
				Column: 15,
				Byte:   14,
			},
		},
		{
			"closing brace",
			hcl.Pos{
				Line:   3,
				Column: 1,
				Byte:   17,
			},
		},
	}

	f, pDiags := hclsyntax.ParseConfig(testConfig, "test.tf", hcl.InitialPos)
	if len(pDiags) > 0 {
		t.Fatal(pDiags)
	}

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			_, err := d.CompletionAtPos(ctx, "test.tf", tc.pos)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), `position outside of "myblock" body`) {
				t.Fatalf("unexpected error message: %q", err.Error())
			}
		})
	}
}

func TestDecoder_CompletionAtPos_rightHandSide(t *testing.T) {
	ctx := context.Background()
	resourceLabelSchema := []*schema.LabelSchema{
		{Name: "type"},
	}
	blockSchema := &schema.BlockSchema{
		Labels: resourceLabelSchema,
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"num_attr": {Constraint: schema.LiteralType{Type: cty.Number}},
				"str_attr": {Constraint: schema.LiteralType{Type: cty.String}},
			},
		},
	}
	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"myblock": blockSchema,
		},
	}
	testConfig := []byte(`myblock "foo" {
  num_attr = 
}
`)

	f, _ := hclsyntax.ParseConfig(testConfig, "test.tf", hcl.InitialPos)

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	candidates, err := d.CompletionAtPos(ctx, "test.tf", hcl.Pos{
		Line:   2,
		Column: 13,
		Byte:   28,
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedCandidates := lang.CompleteCandidates([]lang.Candidate{})
	if diff := cmp.Diff(expectedCandidates, candidates); diff != "" {
		t.Fatalf("unexpected candidates: %s", diff)
	}
}

func TestDecoder_CompletionAtPos_rightHandSideInString(t *testing.T) {
	ctx := context.Background()
	resourceLabelSchema := []*schema.LabelSchema{
		{Name: "type"},
	}
	blockSchema := &schema.BlockSchema{
		Labels: resourceLabelSchema,
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"num_attr": {Constraint: schema.LiteralType{Type: cty.Number}},
				"str_attr": {Constraint: schema.LiteralType{Type: cty.String}},
			},
		},
	}
	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"myblock": blockSchema,
		},
	}
	testConfig := []byte(`myblock "foo" {
  str_attr = ""
}
`)

	f, _ := hclsyntax.ParseConfig(testConfig, "test.tf", hcl.InitialPos)

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	candidates, err := d.CompletionAtPos(ctx, "test.tf", hcl.Pos{
		Line:   2,
		Column: 15,
		Byte:   30,
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedCandidates := lang.CompleteCandidates([]lang.Candidate{})
	if diff := cmp.Diff(expectedCandidates, candidates); diff != "" {
		t.Fatalf("unexpected candidates: %s", diff)
	}
}

func TestDecoder_CompletionAtPos_endOfLabel(t *testing.T) {
	ctx := context.Background()
	blockSchema := &schema.BlockSchema{
		Labels: []*schema.LabelSchema{
			{Name: "type", Completable: true},
		},
		DependentBody: map[schema.SchemaKey]*schema.BodySchema{
			schema.NewSchemaKey(schema.DependencyKeys{
				Labels: []schema.LabelDependent{
					{Index: 0, Value: "myfirst"},
				},
			}): {
				Attributes: map[string]*schema.AttributeSchema{
					"one":   {Constraint: schema.LiteralType{Type: cty.String}},
					"two":   {Constraint: schema.LiteralType{Type: cty.Number}},
					"three": {Constraint: schema.LiteralType{Type: cty.Bool}},
				},
			},
			schema.NewSchemaKey(schema.DependencyKeys{
				Labels: []schema.LabelDependent{
					{Index: 0, Value: "mysecond"},
				},
			}): {
				Attributes: map[string]*schema.AttributeSchema{
					"four": {Constraint: schema.LiteralType{Type: cty.Number}},
					"five": {Constraint: schema.LiteralType{Type: cty.DynamicPseudoType}},
				},
			},
		},
	}
	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"myblock": blockSchema,
		},
	}
	testConfig := []byte(`myblock "my" {
}
`)

	f, _ := hclsyntax.ParseConfig(testConfig, "test.tf", hcl.InitialPos)

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	candidates, err := d.CompletionAtPos(ctx, "test.tf", hcl.Pos{
		Line:   1,
		Column: 12,
		Byte:   11,
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedCandidates := lang.CompleteCandidates([]lang.Candidate{
		{
			Label: "myfirst",
			TextEdit: lang.TextEdit{
				Range: hcl.Range{
					Filename: "test.tf",
					Start: hcl.Pos{
						Line:   1,
						Column: 10,
						Byte:   9,
					},
					End: hcl.Pos{
						Line:   1,
						Column: 12,
						Byte:   11,
					},
				},
				NewText: "myfirst",
				Snippet: "myfirst",
			},
			Kind: lang.LabelCandidateKind,
		},
		{
			Label: "mysecond",
			TextEdit: lang.TextEdit{
				Range: hcl.Range{
					Filename: "test.tf",
					Start: hcl.Pos{
						Line:   1,
						Column: 10,
						Byte:   9,
					},
					End: hcl.Pos{
						Line:   1,
						Column: 12,
						Byte:   11,
					},
				},
				NewText: "mysecond",
				Snippet: "mysecond",
			},
			Kind: lang.LabelCandidateKind,
		},
	})
	if diff := cmp.Diff(expectedCandidates, candidates); diff != "" {
		t.Fatalf("unexpected candidates: %s", diff)
	}
}

func TestDecoder_CompletionAtPos_nonCompletableLabel(t *testing.T) {
	ctx := context.Background()
	blockSchema := &schema.BlockSchema{
		Labels: []*schema.LabelSchema{
			{Name: "type", IsDepKey: true},
		},
		DependentBody: map[schema.SchemaKey]*schema.BodySchema{
			schema.NewSchemaKey(schema.DependencyKeys{
				Labels: []schema.LabelDependent{
					{Index: 0, Value: "myfirst"},
				},
			}): {},
			schema.NewSchemaKey(schema.DependencyKeys{
				Labels: []schema.LabelDependent{
					{Index: 0, Value: "mysecond"},
				},
			}): {},
		},
	}
	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"myblock": blockSchema,
		},
	}
	testConfig := []byte(`myblock "" {
}
`)

	f, _ := hclsyntax.ParseConfig(testConfig, "test.tf", hcl.InitialPos)

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	candidates, err := d.CompletionAtPos(ctx, "test.tf", hcl.Pos{
		Line:   1,
		Column: 10,
		Byte:   9,
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedCandidates := lang.ZeroCandidates()
	if diff := cmp.Diff(expectedCandidates, candidates); diff != "" {
		t.Fatalf("unexpected candidates: %s", diff)
	}
}

func TestDecoder_CompletionAtPos_zeroByteContent(t *testing.T) {
	ctx := context.Background()
	resourceLabelSchema := []*schema.LabelSchema{
		{Name: "type", IsDepKey: true, Completable: true},
		{Name: "name"},
	}
	resourceSchema := &schema.BlockSchema{
		Labels: resourceLabelSchema,
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"count": {Constraint: schema.LiteralType{Type: cty.Number}},
			},
		},
	}
	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"resource": resourceSchema,
		},
	}

	f, pDiags := hclsyntax.ParseConfig([]byte{}, "test.tf", hcl.InitialPos)
	if len(pDiags) > 0 {
		t.Fatal(pDiags)
	}

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	candidates, err := d.CompletionAtPos(ctx, "test.tf", hcl.InitialPos)
	if err != nil {
		t.Fatal(err)
	}
	expectedCandidates := lang.CompleteCandidates([]lang.Candidate{
		{
			Label:  "resource",
			Detail: "Block",
			TextEdit: lang.TextEdit{
				Range: hcl.Range{
					Filename: "test.tf",
					Start:    hcl.InitialPos,
					End:      hcl.InitialPos,
				},
				NewText: "resource",
				Snippet: "resource \"${1}\" \"${2:name}\" {\n  ${3}\n}",
			},
			Kind:           lang.BlockCandidateKind,
			TriggerSuggest: true,
		},
	})
	if diff := cmp.Diff(expectedCandidates, candidates); diff != "" {
		t.Fatalf("unexpected candidates: %s", diff)
	}
}

func TestDecoder_CompletionAtPos_endOfFilePos(t *testing.T) {
	ctx := context.Background()
	resourceLabelSchema := []*schema.LabelSchema{
		{Name: "type", IsDepKey: true, Completable: true},
		{Name: "name"},
	}
	resourceSchema := &schema.BlockSchema{
		Labels: resourceLabelSchema,
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"count": {Constraint: schema.LiteralType{Type: cty.Number}},
			},
		},
	}
	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"resource": resourceSchema,
		},
	}

	cfg := []byte(`resource "azurerm_subnet" "example" {
  count = 3
}
`)

	f, pDiags := hclsyntax.ParseConfig([]byte(cfg), "test.tf", hcl.InitialPos)
	if len(pDiags) > 0 {
		t.Fatal(pDiags)
	}

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	candidates, err := d.CompletionAtPos(ctx, "test.tf", hcl.Pos{Line: 4, Column: 1, Byte: 52})
	if err != nil {
		t.Fatal(err)
	}
	expectedCandidates := lang.Candidates{
		List: []lang.Candidate{
			{
				Label:  "resource",
				Detail: "Block",
				TextEdit: lang.TextEdit{
					Range: hcl.Range{
						Filename: "test.tf",
						Start:    hcl.Pos{Line: 4, Column: 1, Byte: 52},
						End:      hcl.Pos{Line: 4, Column: 1, Byte: 52},
					},
					NewText: "resource",
					Snippet: "resource \"${1}\" \"${2:name}\" {\n  ${3}\n}",
				},
				Kind:           lang.BlockCandidateKind,
				TriggerSuggest: true,
			},
		},
		IsComplete: true,
	}
	if diff := cmp.Diff(expectedCandidates, candidates); diff != "" {
		t.Fatalf("unexpected candidates: %s", diff)
	}
}

func TestDecoder_CompletionAtPos_emptyLabel(t *testing.T) {
	ctx := context.Background()
	resourceLabelSchema := []*schema.LabelSchema{
		{Name: "type", IsDepKey: true, Completable: true},
		{Name: "name"},
	}
	resourceSchema := &schema.BlockSchema{
		Labels: resourceLabelSchema,
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"count": {Constraint: schema.LiteralType{Type: cty.Number}},
			},
		},
		DependentBody: map[schema.SchemaKey]*schema.BodySchema{
			schema.NewSchemaKey(schema.DependencyKeys{
				Labels: []schema.LabelDependent{
					{Index: 0, Value: "azurerm_subnet"},
				},
			}): {
				Attributes: map[string]*schema.AttributeSchema{
					"one":   {Constraint: schema.LiteralType{Type: cty.String}, IsRequired: true},
					"two":   {Constraint: schema.LiteralType{Type: cty.Number}},
					"three": {Constraint: schema.LiteralType{Type: cty.Bool}},
				},
			},
			schema.NewSchemaKey(schema.DependencyKeys{
				Labels: []schema.LabelDependent{
					{Index: 0, Value: "random_resource"},
				},
			}): {
				Attributes: map[string]*schema.AttributeSchema{
					"four": {Constraint: schema.LiteralType{Type: cty.Number}},
					"five": {Constraint: schema.LiteralType{Type: cty.DynamicPseudoType}},
				},
			},
		},
	}
	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"resource": resourceSchema,
		},
	}

	cfg := []byte(`resource "" "" {
}
`)

	f, pDiags := hclsyntax.ParseConfig([]byte(cfg), "test.tf", hcl.InitialPos)
	if len(pDiags) > 0 {
		t.Fatal(pDiags)
	}

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	candidates, err := d.CompletionAtPos(ctx, "test.tf", hcl.Pos{Line: 1, Column: 11, Byte: 10})
	if err != nil {
		t.Fatal(err)
	}
	expectedCandidates := lang.CompleteCandidates([]lang.Candidate{
		{
			Label: "azurerm_subnet",
			TextEdit: lang.TextEdit{
				Range: hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 11, Byte: 10},
					End:      hcl.Pos{Line: 1, Column: 11, Byte: 10},
				},
				NewText: "azurerm_subnet",
				Snippet: "azurerm_subnet",
			},
			Kind: lang.LabelCandidateKind,
		},
		{
			Label: "random_resource",
			TextEdit: lang.TextEdit{
				Range: hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 11, Byte: 10},
					End:      hcl.Pos{Line: 1, Column: 11, Byte: 10},
				},
				NewText: "random_resource",
				Snippet: "random_resource",
			},
			Kind: lang.LabelCandidateKind,
		},
	})
	if diff := cmp.Diff(expectedCandidates, candidates); diff != "" {
		t.Fatalf("unexpected candidates: %s", diff)
	}
}

func TestDecoder_CompletionAtPos_emptyLabel_duplicateDepKeys(t *testing.T) {
	ctx := context.Background()
	resourceLabelSchema := []*schema.LabelSchema{
		{Name: "type", IsDepKey: true, Completable: true},
		{Name: "name"},
	}
	resourceSchema := &schema.BlockSchema{
		Labels: resourceLabelSchema,
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"count": {Constraint: schema.LiteralType{Type: cty.Number}},
			},
		},
		DependentBody: map[schema.SchemaKey]*schema.BodySchema{
			schema.NewSchemaKey(schema.DependencyKeys{
				Labels: []schema.LabelDependent{
					{Index: 0, Value: "azurerm_subnet"},
				},
			}): {
				Attributes: map[string]*schema.AttributeSchema{
					"one":   {Constraint: schema.LiteralType{Type: cty.String}, IsRequired: true},
					"two":   {Constraint: schema.LiteralType{Type: cty.Number}},
					"three": {Constraint: schema.LiteralType{Type: cty.Bool}},
				},
			},
			schema.NewSchemaKey(schema.DependencyKeys{
				Labels: []schema.LabelDependent{
					{Index: 0, Value: "azurerm_subnet"},
				},
				Attributes: []schema.AttributeDependent{
					{
						Name: "provider",
						Expr: schema.ExpressionValue{
							Address: lang.Address{
								lang.RootStep{Name: "azurerm"},
							},
						},
					},
				},
			}): {
				Attributes: map[string]*schema.AttributeSchema{
					"one":   {Constraint: schema.LiteralType{Type: cty.String}, IsRequired: true},
					"two":   {Constraint: schema.LiteralType{Type: cty.Number}},
					"three": {Constraint: schema.LiteralType{Type: cty.Bool}},
				},
			},
		},
	}
	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"resource": resourceSchema,
		},
	}

	cfg := []byte(`resource "" "" {
}
`)

	f, pDiags := hclsyntax.ParseConfig([]byte(cfg), "test.tf", hcl.InitialPos)
	if len(pDiags) > 0 {
		t.Fatal(pDiags)
	}

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	candidates, err := d.CompletionAtPos(ctx, "test.tf", hcl.Pos{Line: 1, Column: 11, Byte: 10})
	if err != nil {
		t.Fatal(err)
	}
	expectedCandidates := lang.CompleteCandidates([]lang.Candidate{
		{
			Label: "azurerm_subnet",
			TextEdit: lang.TextEdit{
				Range: hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 1, Column: 11, Byte: 10},
					End:      hcl.Pos{Line: 1, Column: 11, Byte: 10},
				},
				NewText: "azurerm_subnet",
				Snippet: "azurerm_subnet",
			},
			Kind: lang.LabelCandidateKind,
		},
	})
	if diff := cmp.Diff(expectedCandidates, candidates); diff != "" {
		t.Fatalf("unexpected candidates: %s", diff)
	}
}

func TestDecoder_CompletionAtPos_basic(t *testing.T) {
	ctx := context.Background()
	resourceLabelSchema := []*schema.LabelSchema{
		{Name: "type", IsDepKey: true, Completable: true},
		{Name: "name"},
	}

	resourceSchema := &schema.BlockSchema{
		Labels: resourceLabelSchema,
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"count": {Constraint: schema.LiteralType{Type: cty.Number}},
			},
		},
		DependentBody: map[schema.SchemaKey]*schema.BodySchema{
			schema.NewSchemaKey(schema.DependencyKeys{
				Labels: []schema.LabelDependent{
					{Index: 0, Value: "azurerm_subnet"},
				},
			}): {
				Attributes: map[string]*schema.AttributeSchema{
					"one":   {Constraint: schema.LiteralType{Type: cty.String}, IsRequired: true},
					"two":   {Constraint: schema.LiteralType{Type: cty.Number}, IsOptional: true},
					"three": {Constraint: schema.LiteralType{Type: cty.Bool}, IsOptional: true},
				},
			},
			schema.NewSchemaKey(schema.DependencyKeys{
				Labels: []schema.LabelDependent{
					{Index: 0, Value: "random_resource"},
				},
			}): {
				Attributes: map[string]*schema.AttributeSchema{
					"four": {Constraint: schema.LiteralType{Type: cty.Number}},
					"five": {Constraint: schema.LiteralType{Type: cty.DynamicPseudoType}},
				},
			},
			schema.NewSchemaKey(schema.DependencyKeys{
				Labels: []schema.LabelDependent{
					{Index: 0, Value: "sensitive_resource"},
				},
			}): {
				Attributes: map[string]*schema.AttributeSchema{
					"six":   {Constraint: schema.LiteralType{Type: cty.Number}, IsSensitive: true},
					"seven": {Constraint: schema.LiteralType{Type: cty.Number}, IsRequired: true, IsSensitive: true},
				},
			},
		},
	}

	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"resource": resourceSchema,
		},
	}

	cfg := []byte(`resource "azurerm_subnet" "example" {
  count = 3
}

resource "sensitive_resource" "t" {
  count = 2
}

resource "random_resource" "test" {
  arg = ""
}
`)

	f, pDiags := hclsyntax.ParseConfig(cfg, "test.tf", hcl.InitialPos)
	if len(pDiags) > 0 {
		t.Fatal(pDiags)
	}

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	testCases := []struct {
		name               string
		pos                hcl.Pos
		expectedCandidates lang.Candidates
	}{
		{
			"in-between blocks",
			hcl.Pos{Column: 1, Line: 4, Byte: 52},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "resource",
					Detail: "Block",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 4, Column: 1, Byte: 52},
							End:      hcl.Pos{Line: 4, Column: 1, Byte: 52},
						},
						NewText: "resource",
						Snippet: "resource \"${1}\" \"${2:name}\" {\n  ${3}\n}",
					},
					Kind:           lang.BlockCandidateKind,
					TriggerSuggest: true,
				},
			}),
		},
		{
			"block type",
			hcl.Pos{Line: 1, Column: 2, Byte: 1},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "resource",
					Detail: "Block",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 1, Byte: 0},
							End:      hcl.Pos{Line: 3, Column: 2, Byte: 51},
						},
						NewText: "resource",
						Snippet: "resource \"${1}\" \"${2:name}\" {\n  ${3}\n}",
					},
					Kind:           lang.BlockCandidateKind,
					TriggerSuggest: true,
				},
			}),
		},
		{
			"first label",
			hcl.Pos{Line: 1, Column: 11, Byte: 10},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label: "azurerm_subnet",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 11, Byte: 10},
							End:      hcl.Pos{Line: 1, Column: 25, Byte: 24},
						},
						NewText: "azurerm_subnet",
						Snippet: "azurerm_subnet",
					},
					Kind: lang.LabelCandidateKind,
				},
				{
					Label: "random_resource",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 11, Byte: 10},
							End:      hcl.Pos{Line: 1, Column: 25, Byte: 24},
						},
						NewText: "random_resource",
						Snippet: "random_resource",
					},
					Kind: lang.LabelCandidateKind,
				},
				{
					Label: "sensitive_resource",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 1, Column: 11, Byte: 10},
							End:      hcl.Pos{Line: 1, Column: 25, Byte: 24},
						},
						NewText: "sensitive_resource",
						Snippet: "sensitive_resource",
					},
					Kind: lang.LabelCandidateKind,
				},
			}),
		},
		{
			"first block body",
			hcl.Pos{Line: 2, Column: 1, Byte: 38},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "one",
					Detail: "required, string",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 2, Column: 1, Byte: 38},
							End:      hcl.Pos{Line: 2, Column: 1, Byte: 38},
						},
						NewText: "one",
						Snippet: `one = "${1:value}"`,
					},
					Kind: lang.AttributeCandidateKind,
				},
				{
					Label:  "three",
					Detail: "optional, bool",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 2, Column: 1, Byte: 38},
							End:      hcl.Pos{Line: 2, Column: 1, Byte: 38},
						},
						NewText: "three",
						Snippet: "three = ${1:false}",
					},
					Kind: lang.AttributeCandidateKind,
				},
				{
					Label:  "two",
					Detail: "optional, number",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 2, Column: 1, Byte: 38},
							End:      hcl.Pos{Line: 2, Column: 1, Byte: 38},
						},
						NewText: "two",
						Snippet: "two = ${1:0}",
					},
					Kind: lang.AttributeCandidateKind,
				},
			}),
		},
		{
			"first block attribute",
			hcl.Pos{Line: 2, Column: 3, Byte: 40},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "one",
					Detail: "required, string",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 2, Column: 3, Byte: 40},
							End:      hcl.Pos{Line: 2, Column: 12, Byte: 49},
						},
						NewText: "one",
						Snippet: `one = "${1:value}"`,
					},
					Kind: lang.AttributeCandidateKind,
				},
				{
					Label:  "three",
					Detail: "optional, bool",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 2, Column: 3, Byte: 40},
							End:      hcl.Pos{Line: 2, Column: 12, Byte: 49},
						},
						NewText: "three",
						Snippet: "three = ${1:false}",
					},
					Kind: lang.AttributeCandidateKind,
				},
				{
					Label:  "two",
					Detail: "optional, number",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 2, Column: 3, Byte: 40},
							End:      hcl.Pos{Line: 2, Column: 12, Byte: 49},
						},
						NewText: "two",
						Snippet: "two = ${1:0}",
					},
					Kind: lang.AttributeCandidateKind,
				},
			}),
		},
		{
			"second block attribute",
			hcl.Pos{Line: 6, Column: 1, Byte: 89},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "seven",
					Detail: "required, sensitive, number",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 6, Column: 1, Byte: 89},
							End:      hcl.Pos{Line: 6, Column: 1, Byte: 89},
						},
						NewText: "seven",
						Snippet: "seven = ${1:0}",
					},
					Kind: lang.AttributeCandidateKind,
				},
				{
					Label:  "six",
					Detail: "sensitive, number",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 6, Column: 1, Byte: 89},
							End:      hcl.Pos{Line: 6, Column: 1, Byte: 89},
						},
						NewText: "six",
						Snippet: "six = ${1:0}",
					},
					Kind: lang.AttributeCandidateKind,
				},
			}),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			candidates, err := d.CompletionAtPos(ctx, "test.tf", tc.pos)
			if err != nil {
				t.Fatal(err)
			}
			expectedCandidates := tc.expectedCandidates

			diff := cmp.Diff(expectedCandidates, candidates, ctydebug.CmpOptions)
			if diff != "" {
				t.Fatalf("unexpected schema for %s: %s", stringPos(tc.pos), diff)
			}
		})
	}
}

func TestDecoder_CompletionAtPos_nil_expr(t *testing.T) {
	ctx := context.Background()

	// provider:: is not a traversal expression, so hcl will return a ExprSyntaxError which needs to be handled
	f, _ := hclsyntax.ParseConfig([]byte(`attr = provider::`), "test.tf", hcl.InitialPos)

	d := testPathDecoder(t, &PathContext{
		Schema: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"attr": {Constraint: schema.AnyExpression{OfType: cty.DynamicPseudoType}},
			},
		},
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	pos := hcl.Pos{Line: 1, Column: 18, Byte: 17}

	candidates, err := d.CompletionAtPos(ctx, "test.tf", pos)
	if err != nil {
		t.Fatal(err)
	}

	expectedCandidates := lang.CompleteCandidates([]lang.Candidate{})

	diff := cmp.Diff(expectedCandidates, candidates, ctydebug.CmpOptions)
	if diff != "" {
		t.Fatalf("unexpected schema for %s: %s", stringPos(pos), diff)
	}

}

func TestDecoder_CompletionAtPos_AnyAttribute(t *testing.T) {
	ctx := context.Background()
	providersSchema := &schema.BlockSchema{
		Body: &schema.BodySchema{
			AnyAttribute: &schema.AttributeSchema{
				Constraint: schema.LiteralType{
					Type: cty.Object(map[string]cty.Type{
						"source":  cty.String,
						"version": cty.String,
					}),
				},
			},
		},
	}

	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"required_providers": providersSchema,
		},
	}

	cfg := []byte(`required_providers {

}
`)

	f, pDiags := hclsyntax.ParseConfig(cfg, "test.tf", hcl.InitialPos)
	if len(pDiags) > 0 {
		t.Fatal(pDiags)
	}

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})
	d.PrefillRequiredFields = true

	pos := hcl.Pos{Line: 2, Column: 1, Byte: 21}
	candidates, err := d.CompletionAtPos(ctx, "test.tf", pos)
	if err != nil {
		t.Fatal(err)
	}
	expectedCandidates := lang.CompleteCandidates([]lang.Candidate{
		{
			Label:  "name",
			Detail: "object",
			TextEdit: lang.TextEdit{
				Range: hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 2, Column: 1, Byte: 21},
					End:      hcl.Pos{Line: 2, Column: 1, Byte: 21},
				},
				NewText: "name",
				Snippet: "name = {\n  source = \"${1:value}\"\n  version = \"${2:value}\"\n}",
			},
			Kind: lang.AttributeCandidateKind,
		},
	})

	diff := cmp.Diff(expectedCandidates, candidates, ctydebug.CmpOptions)
	if diff != "" {
		t.Fatalf("unexpected schema for %s: %s", stringPos(pos), diff)
	}
}

func TestDecoder_CompletionAtPos_multipleTypes(t *testing.T) {
	ctx := context.Background()
	resourceLabelSchema := []*schema.LabelSchema{
		{Name: "type", IsDepKey: true, Completable: true},
		{Name: "name"},
	}

	resourceSchema := &schema.BlockSchema{
		Labels: resourceLabelSchema,
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"for_each": {
					Constraint: schema.OneOf{
						schema.LiteralType{Type: cty.Set(cty.DynamicPseudoType)},
						schema.LiteralType{Type: cty.Map(cty.DynamicPseudoType)},
					},
					IsOptional: true,
				},
			},
		},
	}

	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"resource": resourceSchema,
		},
	}

	cfg := []byte(`resource "azurerm_subnet" "example" {

}
`)

	f, pDiags := hclsyntax.ParseConfig(cfg, "test.tf", hcl.InitialPos)
	if len(pDiags) > 0 {
		t.Fatal(pDiags)
	}

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	pos := hcl.Pos{Line: 2, Column: 1, Byte: 38}
	candidates, err := d.CompletionAtPos(ctx, "test.tf", pos)
	if err != nil {
		t.Fatal(err)
	}
	expectedCandidates := lang.CompleteCandidates([]lang.Candidate{
		{
			Label:  "for_each",
			Detail: "optional, set of any single type or map of any single type",
			TextEdit: lang.TextEdit{
				Range: hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 2, Column: 1, Byte: 38},
					End:      hcl.Pos{Line: 2, Column: 1, Byte: 38},
				},
				NewText: "for_each",
				Snippet: "for_each = [ ${1} ]",
			},
			Kind: lang.AttributeCandidateKind,
		},
	})

	diff := cmp.Diff(expectedCandidates, candidates, ctydebug.CmpOptions)
	if diff != "" {
		t.Fatalf("unexpected schema for %s: %s", stringPos(pos), diff)
	}
}

func TestDecoder_CompletionAtPos_incompleteAttrOrBlock(t *testing.T) {
	ctx := context.Background()
	resourceLabelSchema := []*schema.LabelSchema{
		{Name: "type"},
		{Name: "name"},
	}

	resourceSchema := &schema.BlockSchema{
		Labels: resourceLabelSchema,
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"count": {Constraint: schema.LiteralType{Type: cty.Number}, IsOptional: true},
			},
		},
	}

	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"resource": resourceSchema,
		},
	}

	testCases := []struct {
		name               string
		src                string
		pos                hcl.Pos
		expectedCandidates lang.Candidates
	}{
		{
			"new root block or attribute",
			`
res
`,
			hcl.Pos{Line: 2, Column: 4, Byte: 4},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "resource",
					Detail: "Block",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 2, Column: 1, Byte: 1},
							End:      hcl.Pos{Line: 2, Column: 4, Byte: 4},
						},
						NewText: "resource",
						Snippet: "resource \"${1:type}\" \"${2:name}\" {\n  ${3}\n}",
					},
					Kind: lang.BlockCandidateKind,
				},
			}),
		},
		{
			"new block or attribute inside a block",
			`
resource "any" "ref" {
  co
}
`,
			hcl.Pos{Line: 3, Column: 5, Byte: 28},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "count",
					Detail: "optional, number",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 3, Column: 3, Byte: 26},
							End:      hcl.Pos{Line: 3, Column: 5, Byte: 28},
						},
						NewText: "count",
						Snippet: "count = ${1:0}",
					},
					Kind: lang.AttributeCandidateKind,
				},
			}),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			f, _ := hclsyntax.ParseConfig([]byte(tc.src), "test.tf", hcl.InitialPos)

			d := testPathDecoder(t, &PathContext{
				Schema: bodySchema,
				Files: map[string]*hcl.File{
					"test.tf": f,
				},
			})

			candidates, err := d.CompletionAtPos(ctx, "test.tf", tc.pos)
			if err != nil {
				t.Fatal(err)
			}
			expectedCandidates := tc.expectedCandidates

			diff := cmp.Diff(expectedCandidates, candidates, ctydebug.CmpOptions)
			if diff != "" {
				t.Fatalf("unexpected schema for %s: %s", stringPos(tc.pos), diff)
			}
		})
	}
}

func TestDecoder_CompletionAtPos_incompleteLabel(t *testing.T) {
	ctx := context.Background()
	resourceLabelSchema := []*schema.LabelSchema{
		{Name: "type"},
		{Name: "name"},
	}

	resourceSchema := &schema.BlockSchema{
		Labels: resourceLabelSchema,
		Body: &schema.BodySchema{
			Attributes: map[string]*schema.AttributeSchema{
				"foo": {
					IsOptional: true,
					Constraint: schema.LiteralType{Type: cty.String},
				},
			},
		},
		DependentBody: map[schema.SchemaKey]*schema.BodySchema{
			schema.NewSchemaKey(schema.DependencyKeys{
				Labels: []schema.LabelDependent{
					{Index: 0, Value: "mycloud_instance"},
				},
			}): {},
			schema.NewSchemaKey(schema.DependencyKeys{
				Labels: []schema.LabelDependent{
					{Index: 0, Value: "mycloud_bucket"},
				},
			}): {},
		},
	}

	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"resource": resourceSchema,
		},
	}

	testCases := []struct {
		name               string
		src                string
		pos                hcl.Pos
		expectedCandidates lang.Candidates
	}{
		{
			"",
			`
res
`,
			hcl.Pos{Line: 2, Column: 4, Byte: 4},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "resource",
					Detail: "Block",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 2, Column: 1, Byte: 1},
							End:      hcl.Pos{Line: 2, Column: 4, Byte: 4},
						},
						NewText: "resource",
						Snippet: "resource \"${1:type}\" \"${2:name}\" {\n  ${3}\n}",
					},
					Kind: lang.BlockCandidateKind,
				},
			}),
		},
		{
			"new block or attribute inside a block",
			`
resource "any" "ref" {
  fo
}
`,
			hcl.Pos{Line: 3, Column: 5, Byte: 28},
			lang.CompleteCandidates([]lang.Candidate{
				{
					Label:  "foo",
					Detail: "optional, string",
					TextEdit: lang.TextEdit{
						Range: hcl.Range{
							Filename: "test.tf",
							Start:    hcl.Pos{Line: 3, Column: 3, Byte: 26},
							End:      hcl.Pos{Line: 3, Column: 5, Byte: 28},
						},
						NewText: "foo",
						Snippet: `foo = "${1:value}"`,
					},
					Kind: lang.AttributeCandidateKind,
				},
			}),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			f, _ := hclsyntax.ParseConfig([]byte(tc.src), "test.tf", hcl.InitialPos)

			d := testPathDecoder(t, &PathContext{
				Schema: bodySchema,
				Files: map[string]*hcl.File{
					"test.tf": f,
				},
			})

			candidates, err := d.CompletionAtPos(ctx, "test.tf", tc.pos)
			if err != nil {
				t.Fatal(err)
			}
			expectedCandidates := tc.expectedCandidates

			diff := cmp.Diff(expectedCandidates, candidates, ctydebug.CmpOptions)
			if diff != "" {
				t.Fatalf("unexpected schema for %s: %s", stringPos(tc.pos), diff)
			}
		})
	}
}

func TestDecoder_CompletionAtPos_withLexerErrors(t *testing.T) {
	ctx := context.Background()

	bSchema := &schema.BlockSchema{
		Body: &schema.BodySchema{
			Blocks: map[string]*schema.BlockSchema{
				"child": {
					Body: &schema.BodySchema{
						Attributes: map[string]*schema.AttributeSchema{
							"src": {
								IsRequired: true,
								Constraint: schema.LiteralType{Type: cty.String},
							},
						},
						Blocks: map[string]*schema.BlockSchema{
							"repo": {
								Body: &schema.BodySchema{
									Attributes: map[string]*schema.AttributeSchema{
										"name": {
											IsRequired: true,
											Constraint: schema.LiteralType{Type: cty.String},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	bodySchema := &schema.BodySchema{
		Blocks: map[string]*schema.BlockSchema{
			"res": bSchema,
		},
	}

	src := `res {
    child {
        s
        repo {
            name = "unclosed string
        }
    }
}`

	f, _ := hclsyntax.ParseConfig([]byte(src), "test.tf", hcl.InitialPos)

	d := testPathDecoder(t, &PathContext{
		Schema: bodySchema,
		Files: map[string]*hcl.File{
			"test.tf": f,
		},
	})

	pos := hcl.Pos{Line: 3, Column: 9, Byte: 27}
	candidates, err := d.CompletionAtPos(ctx, "test.tf", pos)
	if err != nil {
		t.Fatal(err)
	}

	expectedCandidates := lang.CompleteCandidates([]lang.Candidate{
		{
			Label:  "src",
			Detail: "required, string",
			Kind:   lang.AttributeCandidateKind,
			TextEdit: lang.TextEdit{
				Range: hcl.Range{
					Filename: "test.tf",
					Start:    hcl.Pos{Line: 3, Column: 9, Byte: 26},
					End:      hcl.Pos{Line: 3, Column: 10, Byte: 27},
				},
				NewText: "src",
				Snippet: `src = "${1:value}"`,
			},
		},
	})

	diff := cmp.Diff(expectedCandidates, candidates, ctydebug.CmpOptions)
	if diff != "" {
		t.Fatalf("unexpected candidates: %s", diff)
	}
}

var testConfig = []byte(`resource "azurerm_subnet" "example" {
  count = 3
}

resource "random_resource" "test" {
  arg = ""
}
`)
