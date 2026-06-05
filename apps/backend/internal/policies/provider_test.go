package policies

import (
	"encoding/json"
	"testing"
)

func TestPolicyFromEntry(t *testing.T) {
	doc := json.RawMessage(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::photos/*"]}]}`)
	p := policyFromEntry("my-custom", doc)
	if p.Name != "my-custom" || p.Origin != OriginCustom || !p.Editable {
		t.Fatalf("classification wrong: %+v", p)
	}
	if p.StatementSummary == "" {
		t.Error("expected a non-empty statement summary")
	}
}

func TestStatementSummaryAllowGet(t *testing.T) {
	doc := json.RawMessage(`{"Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::photos/*"}]}`)
	got := statementSummary(doc)
	want := "Allow s3:GetObject on arn:aws:s3:::photos/*"
	if got != want {
		t.Errorf("statementSummary = %q want %q", got, want)
	}
}

func TestStatementSummaryMultipleStatements(t *testing.T) {
	doc := json.RawMessage(`{"Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::a/*"},{"Effect":"Deny","Action":"s3:PutObject","Resource":"arn:aws:s3:::b/*"}]}`)
	got := statementSummary(doc)
	want := "Allow s3:GetObject on arn:aws:s3:::a/* (+1 more)"
	if got != want {
		t.Errorf("statementSummary = %q want %q", got, want)
	}
}

func TestStatementSummaryNoAction(t *testing.T) {
	doc := json.RawMessage(`{"Statement":[{"Effect":"Allow","Action":"","Resource":"arn:aws:s3:::a/*"}]}`)
	got := statementSummary(doc)
	want := "Allow (no action) on arn:aws:s3:::a/*"
	if got != want {
		t.Errorf("statementSummary = %q want %q", got, want)
	}
}

func TestStatementSummaryNoResource(t *testing.T) {
	doc := json.RawMessage(`{"Statement":[{"Effect":"Allow","Action":"s3:GetObject"}]}`)
	got := statementSummary(doc)
	want := "Allow s3:GetObject"
	if got != want {
		t.Errorf("statementSummary = %q want %q", got, want)
	}
}

func TestStatementSummaryEmpty(t *testing.T) {
	doc := json.RawMessage(`{"Statement":[]}`)
	got := statementSummary(doc)
	if got != "" {
		t.Errorf("expected empty summary for empty statements, got %q", got)
	}
}

func TestStatementSummaryBadJSON(t *testing.T) {
	doc := json.RawMessage(`not valid json`)
	got := statementSummary(doc)
	if got != "" {
		t.Errorf("expected empty summary for bad JSON, got %q", got)
	}
}

func TestPolicyFromEntryBuiltin(t *testing.T) {
	doc := json.RawMessage(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`)
	p := policyFromEntry("readonly", doc)
	if p.Origin != OriginBuiltin {
		t.Fatalf("expected OriginBuiltin, got %q", p.Origin)
	}
	if p.Editable {
		t.Error("builtin policy must not be editable")
	}
}

func TestPolicyFromEntryTemplate(t *testing.T) {
	doc := json.RawMessage(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`)
	p := policyFromEntry("harbormaster-read-only", doc)
	if p.Origin != OriginTemplate {
		t.Fatalf("expected OriginTemplate, got %q", p.Origin)
	}
	if p.Editable {
		t.Error("template policy must not be editable")
	}
}
