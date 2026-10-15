package main

import "strings"

// 离线演示数据：字段名与 free5GC main 分支真实响应严格一致
// （Nrf_NFMgmt_NFProfile / producer/oam.go OAMUeContext / nsmf-oam 视图），
// 保证 mock ↔ live 对前端同构。

func inst(id, nfType, ip string, services []string, extra map[string]any) map[string]any {
	svcs := []any{}
	for _, s := range services {
		svcs = append(svcs, map[string]any{
			"serviceName": s, "serviceInstanceId": id + "-" + s,
			"versions": []any{map[string]any{
				"apiVersionInUri": "v1", "apiPrefix": "/" + s + "/v1",
				"uriScheme": "http", "nfServiceStatus": "REGISTERED"}},
			"ipEndPoints": []any{map[string]any{"ipv4Address": ip, "port": float64(8000),
				"transport": "TCP", "networkInstance": ""}},
			"nfServiceStatus": "REGISTERED", "capability": nil, "mUEventPort": 0, "mUEvent": "", "fqdn": "", "apiOwnerId": "",
		})
	}
	p := map[string]any{
		"nfInstanceId": id, "nfType": nfType, "nfStatus": "REGISTERED",
		"ipv4Addresses": []any{ip}, "fqdn": strings.ToLower(nfType) + ".local",
		"recoveryTimeCreateTime": "2026-09-14T06:07:31.262028Z",
		"allowedNfTypes":         []any{}, "priority": 0, "capacity": 100, "load": 32,
		"nfServices": svcs,
	}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

var svcSets = map[string][]string{
	"NRF":  {"nnrf-nfm", "nnrf-disc"},
	"AMF":  {"namf-comm", "namf-evts", "namf-mt", "namf-loc", "namf-oam"},
	"SMF":  {"nsmf-pdusession", "nsmf-event-exposure", "nsmf-oam"},
	"UDM":  {"nudm-sdm", "nudm-uecm", "nudm-ueau", "nudm-ee"},
	"UDR":  {"nudr-dr"},
	"PCF":  {"npcf-smpolicycontrol", "npcf-ampolicycontrol", "npcf-policyauthorization"},
	"NSSF": {"nnssf-nsselection", "nnssf-nssaiavailability"},
	"AUSF": {"nausf-auth"},
	"CHF":  {"nchf-convergedcharging"},
	"BSF":  {"nbsf-management"},
}

var ipOf = map[string]string{
	"NRF": "127.0.0.10", "AMF": "127.0.0.18", "SMF": "127.0.0.2", "UDM": "127.0.0.3",
	"UDR": "127.0.0.4", "PCF": "127.0.0.7", "AUSF": "127.0.0.9", "NSSF": "127.0.0.31",
	"CHF": "127.0.0.113", "BSF": "127.0.0.15",
}

func mockInstances() []map[string]any {
	out := []map[string]any{}
	i := 1
	for t, ip := range ipOf {
		extra := map[string]any{}
		if t == "AMF" {
			extra["amfInfo"] = map[string]any{
				"amfSetId": "001", "amfRegionId": "a",
				"guamiList": []any{map[string]any{"plmnId": map[string]any{"mcc": "208", "mnc": "93"},
					"amfId": "cafe00"}},
				"taiList": []any{map[string]any{"plmnId": map[string]any{"mcc": "208", "mnc": "93"},
					"tac": "000001"}}}
			extra["snssais"] = []any{map[string]any{"sst": float64(1), "sd": "010203"}}
			extra["accessType"] = []any{"3GPP_ACCESS", "NON_3_GPP_ACCESS"}
		}
		if t == "SMF" {
			extra["smfInfo"] = map[string]any{"sNssaiInfoList": []any{map[string]any{
				"dnnList": []any{"internet"}}}}
			extra["snssais"] = []any{map[string]any{"sst": float64(1), "sd": "010203"}}
		}
		if t == "UDR" {
			extra["udrInfo"] = map[string]any{
				"supportedDataSets": []any{"SUBSCRIPTION", "POLICY"}}
		}
		uuid := fmtID(i)
		out = append(out, inst(uuid, t, ip, svcSets[t], extra))
		i++
	}
	return out
}

func fmtID(i int) string {
	h := []string{"a1a1a1a1", "b2b2b2b2", "c3c3c3c3", "d4d4d4d4", "e5e5e5e5",
		"f6f6f6f6", "0a0a0a0a", "1b1b1b1b", "2c2c2c2c", "3d3d3d3d"}
	return h[(i-1)%len(h)] + "-0000-4000-8000-0000000000" + itoaPad(i)
}
func itoaPad(i int) string {
	c := []rune("0123456789")
	return string([]rune{c[(i/10)%10], c[i%10]})
}

func mockUEs() []map[string]any {
	return []map[string]any{
		{
			"supi": "imsi-208930000000001", "cmState": "CM_CONNECTED",
			"accessType": "3GPP_ACCESS", "pei": "imei-867530911111111",
			"ratType":            "NR",
			"servingNetworkName": "5g:mnc093.mcc208.3gppnetwork.org",
			"ulCount":            7, "dlCount": 7, "authMethod": "5G_AKA",
			"isPeiUnauthenticated":      false,
			"integrityEnabledAlgorithm": "NIA2", "cipheringEnabledAlgorithm": "NEA2",
			"amf": "208-93/cafe00",
			"pduSessions": []any{map[string]any{
				"pduSessionId": 1,
				"smContextRef": "urn:uuid:00000000-0000-4000-8000-000000000002",
				"smfUri":       "http://127.0.0.2:8000/nsmf-pdusession/v1", "cnid": 7,
			}},
		},
		{
			"supi": "imsi-208930000000041", "cmState": "CM_IDLE",
			"accessType": "3GPP_ACCESS", "pei": "imei-867530922222222",
			"ratType":            "NR",
			"servingNetworkName": "5g:mnc093.mcc208.3gppnetwork.org",
			"ulCount":            5224540, "dlCount": 5224540, "authMethod": "EAP_AKA_PRIME",
			"isPeiUnauthenticated":      false,
			"integrityEnabledAlgorithm": "NIA0", "cipheringEnabledAlgorithm": "NEA0",
			"amf": "208-93/cafe00", "pduSessions": []any{},
		},
	}
}

func mockSession(ref string) map[string]any {
	if !strings.Contains(ref, "00000002") {
		return nil
	}
	return map[string]any{
		"supi": "imsi-208930000000001", "pduSessionID": 1,
		"smContextRef": ref, "dnn": "internet",
		"sNssai":      map[string]any{"sst": 1, "sd": "010203"},
		"ueIP":        "60.60.0.101",
		"pduAddress":  "60.60.0.101",
		"selectedUPF": "UPF", "upCnxState": "UP_ACTIVE",
		"anNodeAddr": "127.0.0.1", "upN3TeID": 114, "cpN4TeID": 147,
		"localSEID": "0x0000000000000001", "remoteSEID": "0x0808080800000002",
		"amfIPv4": "127.0.0.18", "smfIPv4": "127.0.0.2", "pcfIPv4": "127.0.0.7",
		"sessionRule": map[string]any{"sID": "internet", "defQER": true,
			"defUpQoSRule": true, "defQosQFI": 1, "chgLevel": "DNN",
			"authSessAMBR": map[string]any{"uplink": "7000 kbps", "downlink": "7000 kbps"}},
		"qosFlows": []any{map[string]any{"5qi": 9, "qfi": 1, "defQosFlow": true,
			"arp": map[string]any{"pvi": 2, "preCap": 2, "sca": "FALSE"}}},
	}
}

func mockUserplane() map[string]any {
	return map[string]any{
		"upNodes": map[string]any{
			"AN1": map[string]any{"name": "AN1", "type": "AN",
				"nodeID": map[string]any{"type": 0, "id": "127.0.0.1"},
				"links":  []any{"UPF"}},
			"UPF": map[string]any{"name": "UPF", "type": "UPF",
				"nodeID":   map[string]any{"type": 0, "id": "127.0.0.8"},
				"pfcpAddr": "127.0.0.8:8805",
				"status": map[string]any{"pfcpAssociationSetup": true,
					"associationEstablishedTime": "2026-09-14T06:07:35Z"},
				"snssaiDnnPools": []any{map[string]any{"sst": 1, "sd": "010203",
					"dnn": "internet", "pool": []any{"60.60.0.0/24"}}},
				"links": []any{"AN1"}},
		},
		"upPaths": []any{[]any{"AN1", "UPF"}},
	}
}
