/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	types "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	fed "k8s.io/dns/pkg/dns/federation"
	"k8s.io/dns/pkg/dns/util"
)

// Config populated either from the configuration source (command
// line flags or via the config map mechanism).
type Config struct {
	// The inclusion of TypeMeta is to ensure future compatibility if the
	// Config object was populated directly via a Kubernetes API mechanism.
	//
	// For example, instead of the custom implementation here, the
	// configuration could be obtained from an API that unifies
	// command-line flags, config-map, etc mechanisms.
	types.TypeMeta

	// Map of federation names that the cluster in which this kube-dns
	// is running belongs to, to the corresponding domain names.
	Federations map[string]string `json:"federations"`

	// Map of stub domain to nameserver IP. The key is the domain name suffix,
	// e.g. "acme.local". Key cannot be equal to the cluster domain. Value is
	// the IP of the nameserver to send DNS request for the given subdomain.
	StubDomains map[string][]string `json:"stubDomains"`

	// List of upstream nameservers to use. Overrides nameservers inherited
	// from the node.
	UpstreamNameservers []string `json:"upstreamNameservers"`
}

func NewDefaultConfig() *Config {
	return &Config{
		Federations: map[string]string{},
		StubDomains: map[string][]string{},
	}
}

// Validate returns whether or not the configuration is valid.
func (config *Config) Validate() error {
	if err := config.validateFederations(); err != nil {
		return err
	}

	if err := config.validateStubDomains(); err != nil {
		return err
	}

	if err := config.validateUpstreamNameserver(); err != nil {
		return err
	}

	return nil
}

func (config *Config) validateFederations() error {
	for name, domain := range config.Federations {
		if err := fed.ValidateName(name); err != nil {
			return err
		}
		if err := fed.ValidateDomain(domain); err != nil {
			return err
		}
	}
	return nil
}

func (config *Config) validateStubDomains() error {
	for domain, nsList := range config.StubDomains {
		if len(validation.IsDNS1123Subdomain(domain)) != 0 {
			return fmt.Errorf("invalid domain name: %q", domain)
		}

		for _, ns := range nsList {
			// TODO(rramkumar): Use net.SplitHostPort to support ipv6 case.
			nsStrings := strings.SplitN(ns, ":", 2)
			// Validate port if specified
			if len(nsStrings) == 2 {
				if _, err := strconv.ParseUint(nsStrings[1], 10, 16); err != nil {
					return fmt.Errorf("invalid nameserver: %q", ns)
				}
			}
			if len(validation.IsValidIP(nsStrings[0])) > 0 && len(validation.IsDNS1123Subdomain(ns)) > 0 {
				return fmt.Errorf("invalid nameserver: %q", ns)
			}
		}
	}

	return nil
}

func (config *Config) validateUpstreamNameserver() error {

	if len(config.UpstreamNameservers) > 3 {
		return fmt.Errorf("upstreamNameserver cannot have more than three entries")
	}

	for _, nameServer := range config.UpstreamNameservers {
		if _, _, err := util.ValidateNameserverIpAndPort(nameServer); err != nil {
			return err
		}
	}
	return nil
}

// Config contains a configmap name, a config directory and interval of how often to poll config.

type DNSConfig struct {
	ClusterDomain      string
	KubeConfigFile     string
	KubeMasterURL      string
	InitialSyncTimeout time.Duration

	HealthzPort    int
	DNSBindAddress string
	DNSPort        int

	Federations map[string]string

	ConfigMapNs string
	ConfigMap   string

	ConfigDir    string
	ConfigPeriod time.Duration

	NameServers string
}

func NewDNSConfig() *DNSConfig {
	return &DNSConfig{
		ClusterDomain:      "cluster.local.",
		HealthzPort:        8081,
		DNSBindAddress:     "0.0.0.0",
		DNSPort:            53,
		InitialSyncTimeout: 60 * time.Second,

		Federations: make(map[string]string),

		ConfigMapNs: api.NamespaceSystem,
		ConfigMap:   "", // default to using command line flags

		ConfigPeriod: 10 * time.Second,
		ConfigDir:    "",
		NameServers:  "",
	}
}

func NewConfigSync(kubeClient kubernetes.Interface, config *DNSConfig) Sync {

	var configSync Sync
	switch {
	case config.ConfigMap != "" && config.ConfigDir != "":
		glog.Fatal("Cannot use both ConfigMap and ConfigDir")

	case config.ConfigMap != "":
		glog.V(0).Infof("Using configuration read from ConfigMap: %v:%v", config.ConfigMapNs, config.ConfigMap)
		configSync = NewConfigMapSync(kubeClient, config.ConfigMapNs, config.ConfigMap)

	case config.ConfigDir != "":
		glog.V(0).Infof("Using configuration read from directory: %v with period %v", config.ConfigDir, config.ConfigPeriod)
		configSync = NewFileSync(config.ConfigDir, config.ConfigPeriod)

	default:
		glog.V(0).Infof("ConfigMap and ConfigDir not configured, using values from command line flags")
		conf := Config{Federations: config.Federations}
		if len(config.NameServers) > 0 {
			conf.UpstreamNameservers = strings.Split(config.NameServers, ",")
		}
		configSync = NewNopSync(&conf)
	}
	return configSync
}

func StartConfigMapSync(configSync *Sync, updateFunc func(*Config), periodicSync func(<-chan *Config)) error {
	if configSync == nil {
		return fmt.Errorf("Invalid configmap specified")
	}
	initialConfig, err := (*configSync).Once()
	if err != nil {
		return err
	} else {
		updateFunc(initialConfig)
	}
	go periodicSync((*configSync).Periodic())
	return nil
}
