package main

import "testing"

func TestFormatHTTPDSLSource_Indentation(t *testing.T) {
	in := `server {
port 8080
route GET "/" {
if true {
response.body = { ok: true}
} else {
response.body = { ok: false}
}
}
}
`

	got := formatHTTPDSLSource(in)
	want := `server {
    port 8080
    route GET "/" {
        if true {
            response.body = { ok: true}
        } else {
            response.body = { ok: false}
        }
    }
}
`

	if got != want {
		t.Fatalf("formatted output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatHTTPDSLSource_PreservesTemplateBody(t *testing.T) {
	in := "route GET \"/\" {\n" +
		"    response.body = `\n" +
		"  <div>\n" +
		"      <p>${name}</p>\n" +
		"  </div>\n" +
		"`\n" +
		"}\n"

	got := formatHTTPDSLSource(in)
	want := in

	if got != want {
		t.Fatalf("template body should be preserved\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
