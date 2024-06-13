package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	etcdcv3 "go.etcd.io/etcd/client/v3"
	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
)

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	// This will register our custom DNS provider with the webhook serving
	// library, making it available as an API under the provided GroupName.
	// You can register multiple DNS provider implementations with a single
	// webhook, where the Name() method will be used to disambiguate between
	// the different implementations.
	cmd.RunWebhookServer(GroupName,
		&corednsProviderSolver{},
	)
}

type TXTRecord struct {
	Text string `json:"text,omitempty"`
	TTL  uint32 `json:"ttl,omitempty"`
}

// corednsProviderSolver implements the provider-specific logic needed to
// 'present' an ACME challenge TXT record for your own DNS provider.
// To do so, it must implement the `github.com/cert-manager/cert-manager/pkg/acme/webhook.Solver`
// interface.
type corednsProviderSolver struct {
	client *kubernetes.Clientset
}

// customDNSProviderConfig is a structure that is used to decode into when
// solving a DNS01 challenge.
// This information is provided by cert-manager, and may be a reference to
// additional configuration that's needed to solve the challenge for this
// particular certificate or issuer.
// This typically includes references to Secret resources containing DNS
// provider credentials, in cases where a 'multi-tenant' DNS solver is being
// created.
// If you do *not* require per-issuer or per-certificate configuration to be
// provided to your webhook, you can skip decoding altogether in favour of
// using CLI flags or similar to provide configuration.
// You should not include sensitive information here. If credentials need to
// be used by your provider here, you should reference a Kubernetes Secret
// resource and fetch these credentials using a Kubernetes clientset.
type corednsProviderConfig struct {
	// Change the two fields below according to the format of the configuration
	// to be decoded.
	// These fields will be set by users in the
	// `issuer.spec.acme.dns01.providers.webhook.config` field.

	CoreDNSPrefix   string                   `json:"coreDNSPrefix"`
	EtcdEndpoints   string                   `json:"etcdEndpoints"`
	EtcdUsernameRef corev1.SecretKeySelector `json:"etcdUsernameRef"`
	EtcdPasswordRef corev1.SecretKeySelector `json:"etcdPasswordRef"`
}

// Name is used as the name for this DNS solver when referencing it on the ACME
// Issuer resource.
// This should be unique **within the group name**, i.e. you can have two
// solvers configured with the same Name() **so long as they do not co-exist
// within a single webhook deployment**.
// For example, `cloudflare` may be used as the name of a solver.
func (c *corednsProviderSolver) Name() string {
	return "coredns-solver"
}

// Present is responsible for actually presenting the DNS record with the
// DNS provider.
// This method should tolerate being called multiple times with the same value.
// cert-manager itself will later perform a self check to ensure that the
// solver has correctly configured the DNS provider.
func (c *corednsProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	err = validateConfig(&cfg)
	if err != nil {
		return err
	}

	fmt.Println("config", cfg)

	etcdClient, err := c.NewEtcdClient(cfg, ch)
	if err != nil {
		return err
	}

	defer etcdClient.Close()
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	value, err := json.Marshal(TXTRecord{
		Text: ch.Key,
		TTL:  uint32(60),
	})

	if err != nil {
		return err
	}
	key := etcdKeyFor(cfg.CoreDNSPrefix, ch.ResolvedFQDN, ch.Key)
	fmt.Println("key", key)

	_, err = etcdClient.Put(ctx, key, string(value))
	return err
}

// CleanUp should delete the relevant TXT record from the DNS provider console.
// If multiple TXT records exist with the same record name (e.g.
// _acme-challenge.example.com) then **only** the record with the same `key`
// value provided on the ChallengeRequest should be cleaned up.
// This is in order to facilitate multiple DNS validations for the same domain
// concurrently.
func (c *corednsProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	err = validateConfig(&cfg)
	if err != nil {
		return err
	}

	etcdClient, err := c.NewEtcdClient(cfg, ch)
	if err != nil {
		return err
	}

	defer etcdClient.Close()
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	key := etcdKeyFor(cfg.CoreDNSPrefix, ch.ResolvedFQDN, ch.Key)
	_, err = etcdClient.Delete(ctx, key, etcdcv3.WithPrefix())

	return err
}

// Initialize will be called when the webhook first starts.
// This method can be used to instantiate the webhook, i.e. initialising
// connections or warming up caches.
// Typically, the kubeClientConfig parameter is used to build a Kubernetes
// client that can be used to fetch resources from the Kubernetes API, e.g.
// Secret resources containing credentials used to authenticate with DNS
// provider accounts.
// The stopCh can be used to handle early termination of the webhook, in cases
// where a SIGTERM or similar signal is sent to the webhook process.
func (c *corednsProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	client, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}

	c.client = client
	return nil
}

func (c *corednsProviderSolver) NewEtcdClient(cfg corednsProviderConfig, ch *v1alpha1.ChallengeRequest) (*etcdcv3.Client, error) {
	etcdClientConfig, err := c.NewETCDConfig(cfg, ch)
	if err != nil {
		return nil, err
	}
	return etcdcv3.New(*etcdClientConfig)

}
func (c *corednsProviderSolver) NewETCDConfig(cfg corednsProviderConfig, ch *v1alpha1.ChallengeRequest) (*etcdcv3.Config, error) {
	etcdURLs := strings.Split(cfg.EtcdEndpoints, ",")
	etcdUsername, err := c.secret(cfg.EtcdUsernameRef, ch.ResourceNamespace)
	if err != nil {
		return nil, err
	}
	etcdPassword, err := c.secret(cfg.EtcdPasswordRef, ch.ResourceNamespace)
	if err != nil {
		return nil, err
	}
	return &etcdcv3.Config{
		Endpoints: etcdURLs,
		Username:  etcdUsername,
		Password:  etcdPassword,
	}, nil
}

func (c *corednsProviderSolver) secret(ref corev1.SecretKeySelector, namespace string) (string, error) {
	if ref.Name == "" {
		return "", nil
	}

	secret, err := c.client.CoreV1().Secrets(namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	bytes, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key not found %q in secret '%s/%s'", ref.Key, namespace, ref.Name)
	}
	return string(bytes), nil
}

// loadConfig is a small helper function that decodes JSON configuration into
// the typed config struct.
func loadConfig(cfgJSON *extapi.JSON) (corednsProviderConfig, error) {
	cfg := corednsProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}

func validateConfig(cfg *corednsProviderConfig) error {
	if cfg.CoreDNSPrefix == "" {
		return errors.New("no `CoreDNSPrefix` provided")
	}
	if cfg.EtcdEndpoints == "" {
		return errors.New("no `EtcdEndpoints` provided")
	}
	if cfg.EtcdUsernameRef.Name == "" {
		return errors.New("no `EtcdUsernameRef` secret provided")
	}
	if cfg.EtcdPasswordRef.Name == "" {
		return errors.New("no `EtcdPasswordRef` secret provided")
	}
	return nil
}

func etcdKeyFor(prefix, dnsName, key string) string {
	domains := strings.Split(dnsName, ".")
	reverse(domains)
	return prefix + strings.Join(domains, "/") + "/" + key
}

func reverse(slice []string) {
	for i := 0; i < len(slice)/2; i++ {
		j := len(slice) - i - 1
		slice[i], slice[j] = slice[j], slice[i]
	}
}
