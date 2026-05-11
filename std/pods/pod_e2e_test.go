package pods

import (
	"os"
	"testing"

	. "github.com/candid82/joker/core"
)

func TestHelperProcessFormatPod(t *testing.T) {
	if os.Getenv("GO_WANT_FORMAT_POD") == "" {
		return
	}
	format := os.Getenv("GO_WANT_FORMAT_POD")
	for {
		obj, err := bencodeDecodeReader(os.Stdin)
		if err != nil {
			os.Exit(2)
		}
		msg := objectMapToPodMessage(obj)
		id, _ := msg["id"].(string)
		switch msg["op"] {
		case "describe":
			os.Stdout.Write(bencodeEncodePlain(map[string]any{
				"id":     id,
				"format": format,
				"namespaces": []any{map[string]any{
					"name": "fake." + formatName(format),
					"vars": []any{map[string]any{"name": "echo", "doc": "echoes pod args"}},
				}},
				"done": true,
			}))
		case "invoke":
			os.Stdout.Write(bencodeEncodePlain(map[string]any{
				"id":    id,
				"value": msg["args"],
				"done":  true,
			}))
			return
		}
	}
}

func formatName(format string) string {
	switch format {
	case "transit+json":
		return "transit"
	default:
		return format
	}
}

func TestPodEndToEndFormats(t *testing.T) {
	for _, tc := range []struct {
		format string
		ns     string
	}{
		{format: "json", ns: "fake.json"},
		{format: "edn", ns: "fake.edn"},
		{format: "transit+json", ns: "fake.transit"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			shutdownAllPods()
			t.Setenv("GO_WANT_FORMAT_POD", tc.format)
			p, describe, err := startPodProcess(os.Args[0], []string{"-test.run=TestHelperProcessFormatPod"})
			if err != nil {
				t.Fatal(err)
			}
			defer p.shutdownPod()
			if describe["format"] != tc.format {
				t.Fatalf("format mismatch: %#v", describe)
			}
			p.format = tc.format
			registerPod(p)
			if err := installPodDescribeNamespaces(p, describe); err != nil {
				t.Fatal(err)
			}
			ns := GLOBAL_ENV.EnsureSymbolIsNamespace(MakeSymbol(tc.ns))
			vr := ns.Resolve("echo")
			if vr == nil {
				t.Fatalf("dynamic var %s/echo missing", tc.ns)
			}
			callable, ok := vr.Value.(Callable)
			if !ok {
				t.Fatalf("dynamic var is not callable: %T", vr.Value)
			}
			res := callable.Call([]Object{MakeKeyword("k"), MakeInt(7)})
			got := res.ToString(false)
			if got != "[:k 7]" && got != "(:k 7)" {
				t.Fatalf("%s echo = %s", tc.format, got)
			}
		})
	}
}

func TestPodDynamicNamespaceVarSmoke(t *testing.T) {
	shutdownAllPods()
	p := newPod("pod-smoke", "smoke", "json", nil, nil, nil)
	registerPod(p)
	describe := podMessage{"namespaces": []any{podMessage{"name": "fake.smoke", "vars": []any{podMessage{"name": "echo", "doc": "Echo doc"}}}}}
	if err := installPodDescribeNamespaces(p, describe); err != nil {
		t.Fatal(err)
	}
	ns := GLOBAL_ENV.EnsureSymbolIsNamespace(MakeSymbol("fake.smoke"))
	vr := ns.Resolve("echo")
	if vr == nil || vr.Value.GetType() != TYPE.Proc {
		t.Fatalf("bad dynamic var: %#v", vr)
	}
	if ok, doc := vr.GetMeta().Get(MakeKeyword("doc")); !ok || doc.ToString(false) != "Echo doc" {
		t.Fatalf("expected metadata doc, got %#v ok=%v", doc, ok)
	}
}
