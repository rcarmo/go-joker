package pods

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"

	. "github.com/rcarmo/go-joker/core"
)

func installPodDescribeNamespaces(p *Pod, describe podMessage) error {
	nss, ok := describe["namespaces"].([]any)
	if !ok {
		return nil
	}
	for _, rawNS := range nss {
		nsMsg, ok := rawNS.(podMessage)
		if !ok {
			if m, ok := rawNS.(map[string]any); ok {
				nsMsg = podMessage(m)
			} else {
				continue
			}
		}
		name, _ := nsMsg["name"].(string)
		if name == "" {
			continue
		}
		ns := GLOBAL_ENV.EnsureSymbolIsLib(MakeSymbol(name))
		ns.ResetMeta(MakeMeta(nil, "Babashka pod namespace dynamically installed from pod describe.", "1.0"))
		vars, _ := nsMsg["vars"].([]any)
		for _, rawVar := range vars {
			varMsg, ok := rawVar.(podMessage)
			if !ok {
				if m, ok := rawVar.(map[string]any); ok {
					varMsg = podMessage(m)
				} else {
					continue
				}
			}
			varName, _ := varMsg["name"].(string)
			if varName == "" {
				continue
			}
			qualified := name + "/" + varName
			doc := fmt.Sprintf("Proxy for Babashka pod var `%s`.", qualified)
			if metaDoc, ok := varMsg["doc"].(string); ok && metaDoc != "" {
				doc = metaDoc
			}
			ns.InternVar(varName, makePodProxyProc(p, qualified), MakeMeta(nil, doc, "1.0"))
		}
	}
	return nil
}

func makePodProxyProc(p *Pod, qualified string) Proc {
	return Proc{
		Name:    "pod-proxy-" + qualified,
		Package: "std/pods",
		Fn: func(args []coretypes.Object) coretypes.Object {
			res, err := p.invoke(qualified, args)
			if err != nil {
				panic(RT.NewError("pod " + qualified + ": " + err.Error()))
			}
			return res
		},
	}
}
