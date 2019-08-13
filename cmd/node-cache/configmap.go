package main

import (
	"io/ioutil"
	"strings"

	clog "github.com/coredns/coredns/plugin/pkg/log"
	"k8s.io/dns/pkg/dns/config"
)

func (c *cacheApp) updateConfig(config *config.Config) {
	// construct part of the Corefile
	cstr := ""
	for domainName, servers := range config.StubDomains {
		lines := []string{domainName + ":53 {", "\terrors", "\tcache 30", "\tforward . " + servers[0], "}"}
		cstr = cstr + strings.Join(lines, "\n")
	}
	clog.Infof("WILL UPDATE CONFIG WITH %s", cstr)
	baseConfig, err := ioutil.ReadFile(c.params.cmPath)
	if err != nil {
		clog.Errorf("Failed to read node-cache configmap %s - %v", c.params.cmPath, err)
		return
	}
	strings.Replace(string(baseConfig), "STUB_DOMAINS", cstr, -1)
	strings.Replace(string(baseConfig), "UPSTREAM_SERVERS", strings.Join(config.UpstreamNameservers, " "), -1)
	err = ioutil.WriteFile(c.params.confFile, []byte(baseConfig), 0666)
	if err != nil {
		clog.Errorf("Failed to write config file %s - err %v", c.params.confFile, err)
	}
}

func (c *cacheApp) syncConfigMap(syncChan <-chan *config.Config) {
	for {
		nextConfig := <-syncChan
		c.updateConfig(nextConfig)
	}
}

func (c *cacheApp) initConfigMapSync() {
	/*
		kubeClient, err := util.GetDefaultKubeClient("nodelocaldns-%s", version.Version)
		if err != nil {
			glog.Fatalf("Failed to create a kubernetes client: %v", err)
		}
	*/
	c.dnsConfig.ConfigDir = "/etc/kube-dns/"
	configSync := config.NewConfigSync(nil, c.dnsConfig)
	config.StartConfigMapSync(&configSync, c.updateConfig, c.syncConfigMap)
}
