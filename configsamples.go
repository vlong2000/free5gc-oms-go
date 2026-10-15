package main

import (
	"gopkg.in/yaml.v3"
)

// 内置示例配置：与 free5GC 官方 samples 同构（版本号为各 NF govalidator 白名单值），
// 仅在未提供 --config-dir 或 mock 模式用于离线演示；真实运行请用
// --config-dir ~/free5gc/config 读取现场文件。

var sampleYAML = map[string]string{
	"nrf": `
info:
  version: 1.0.2
  description: NRF initial local configuration
configuration:
  nrfName: NRF
  sbi:
    scheme: http
    registerIPv4: 127.0.0.10
    bindingIPv4: 127.0.0.10
    port: 8000
  serviceNameList:
    - nnrf-nfm
    - nnrf-disc
  mongodb:
    name: free5gc
    url: mongodb://localhost:27017
  defaultPlmnId:
    mcc: '208'
    mnc: '93'
`,
	"amf": `
info:
  version: 1.0.9
  description: AMF initial local configuration
configuration:
  amfName: AMF
  ngapIpList:
    - 127.0.0.18
  ngapPort: 38412
  sbi:
    scheme: http
    registerIPv4: 127.0.0.18
    bindingIPv4: 127.0.0.18
    port: 8000
  serviceNameList:
    - namf-comm
    - namf-evts
    - namf-mt
    - namf-loc
    - namf-oam
  servedGuamiList:
    - plmnId:
        mcc: '208'
        mnc: '93'
      amfId: cafe00
  supportTaiList:
    - plmnId:
        mcc: '208'
        mnc: '93'
      tac: '000001'
  plmnSupportList:
    - plmnId:
        mcc: '208'
        mnc: '93'
      snssaiList:
        - sst: 1
          sd: '010203'
        - sst: 1
          sd: '112233'
  supportDnnList:
    - internet
  security:
    integrityOrder:
      - NIA2
    cipheringOrder:
      - NEA0
      - NEA2
  networkName:
    full: free5GC
    short: free
  t3502Value: 720
  t3512Value: 3600
  non3gppDeregTimerValue: 32400
  nrfUri: http://127.0.0.10:8000
  locality: area1
`,
	"smf": `
info:
  version: 1.0.7
  description: SMF initial local configuration
configuration:
  smfName: SMF
  sbi:
    scheme: http
    registerIPv4: 127.0.0.2
    bindingIPv4: 127.0.0.2
    port: 8000
  serviceNameList:
    - nsmf-pdusession
    - nsmf-event-exposure
    - nsmf-oam
  snssaiInfos:
    - sNssaiInfo:
        - sst: 1
          sd: '010203'
      dnnInfos:
        - dnn: internet
          dns:
            ipv4: 8.8.8.8
  plmnList:
    - mcc: '208'
      mnc: '93'
  pfcp:
    nodeID: 127.0.0.1
    listenAddr: 127.0.0.1
    heartbeatInterval: 10s
    assocFailAlertInterval: 10s
    assocFailRetryInterval: 30s
  userplaneInformation:
    upNodes:
      - type: UPF
        name: UPF
        nodeID: 127.0.0.8
        sNssaiUpfInfos:
          - sNssai:
              sst: 1
              sd: '010203'
            dnnUpfInfoList:
              - dnn: internet
                pools:
                  - 10.60.0.0/16
                staticPools:
                  - 10.60.100.0/24
        links: [AN1]
      - type: AN
        name: AN1
        addr: 127.0.0.1
        links: [UPF]
    links:
      - AN: AN1
        UPF: UPF
`,
	"upf": `
version: 1.0.3
description: UPF initial local configuration
pfcp:
  addr: 127.0.0.8
  nodeID: 127.0.0.8
gtpu:
  forwarder: gtp5g
  iflist:
    - addr: 127.0.0.8
      type: N3
dnn_list:
  - dnn: internet
    cidr: 60.60.0.0/24
    natifname: eth0
`,
	"udm": `
info:
  version: 1.0.6
  description: UDM initial local configuration
configuration:
  udmName: UDM
  sbi:
    scheme: http
    registerIPv4: 127.0.0.3
    port: 8000
  serviceNameList:
    - nudm-sdm
    - nudm-uecm
    - nudm-ueau
  nrfUri: http://127.0.0.10:8000
  SuciProfile:
    - protectionScheme: 1
      homeNetworkPublicKey: 5a8d388644379ac68c15392b80149e08c427e10dae41883a721dfa19e6000000
      homeNetworkPublicKeyIdentifier: 0f633fa61e76e27366cbe046ca66e2451fa6babc67d205f210e2948e72b59881
`,
	"udr": `
info:
  version: 1.0.4
  description: UDR initial local configuration
configuration:
  udrName: UDR
  sbi:
    scheme: http
    registerIPv4: 127.0.0.4
    port: 8000
  dbConnectorType: mongodb
  mongodb:
    name: free5gc
    url: mongodb://localhost:27017
  nrfUri: http://127.0.0.10:8000
`,
	"pcf": `
info:
  version: 1.0.7
  description: PCF initial local configuration
configuration:
  pcfName: PCF
  sbi:
    scheme: http
    registerIPv4: 127.0.0.7
    port: 8000
  serviceNameList:
    - npcf-smpolicycontrol
    - npcf-ampolicycontrol
    - npcf-policyauthorization
  nrfUri: http://127.0.0.10:8000
  timeFormat: non-rfc3339
`,
	"nssf": `
info:
  version: 1.0.3
  description: NSSF initial local configuration
configuration:
  nssfName: NSSF
  sbi:
    scheme: http
    registerIPv4: 127.0.0.31
    port: 8000
  serviceNameList:
    - nnssf-nsselection
    - nnssf-nssaiavailability
  nrfUri: http://127.0.0.10:8000
  supportedPlmnList:
    - mcc: '208'
      mnc: '93'
  nsiList:
    - snssai:
        sst: 1
      nsiInformationList:
        - nsiId: 10
          nssiDnnList:
            - internet
`,
	"ausf": `
info:
  version: 1.0.4
  description: AUSF initial local configuration
configuration:
  ausfName: AUSF
  sbi:
    scheme: http
    registerIPv4: 127.0.0.9
    port: 8000
  serviceNameList:
    - nausf-auth
  nrfUri: http://127.0.0.10:8000
  plmnSupportList:
    - mcc: '208'
      mnc: '093'
`,
}

func builtinSampleConfigs() *ConfigSets {
	cs := &ConfigSets{Dir: "", Items: map[string]cfgFile{}}
	for nf, text := range sampleYAML {
		f := cfgFile{Path: "(内置示例) config/" + nf + "cfg.yaml", Raw: text}
		var v map[string]any
		if err := yaml.Unmarshal([]byte(text), &v); err != nil {
			f.Err = err.Error()
		} else {
			f.Data = v
		}
		cs.Items[nf] = f
	}
	return cs
}
