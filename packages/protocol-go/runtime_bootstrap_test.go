package protocol

import (
	"encoding/json"
	"testing"
)

func TestRuntimeBootstrap_RoundTripOmitsNothingStructural(t *testing.T) {
	in := RuntimeBootstrap{
		RevisionID: "rev-1",
		Env: []RuntimeVariable{
			{Name: "REDIS_URL", Value: "redis://redis:6379/1"},
			{Name: "APP_SECRET", Value: "stored-secret"},
		},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out RuntimeBootstrap
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.RevisionID != in.RevisionID || len(out.Env) != 2 {
		t.Fatalf("round trip = %+v", out)
	}
	if out.Env[0].Name != "REDIS_URL" || out.Env[0].Value != "redis://redis:6379/1" {
		t.Fatalf("variable = %+v", out.Env[0])
	}
	if out.Env[1].Name != "APP_SECRET" || out.Env[1].Value != "stored-secret" {
		t.Fatalf("secret = %+v", out.Env[1])
	}
}
