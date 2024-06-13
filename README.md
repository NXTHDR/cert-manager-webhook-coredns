<p align="center">
  <img src="https://raw.githubusercontent.com/cert-manager/cert-manager/d53c0b9270f8cd90d908460d69502694e1838f5f/logo/logo-small.png" height="256" width="256" alt="cert-manager project logo" />
</p>

# CoreDNS ACME webhook

```sh
docker build -t cert-manager-webhook-coredns .
```

Choose a unique group name to identify your company or organization (for example `acme.mycompany.example`).

```sh
helm upgrade --install \
  cert-manager-webhook-coredns \
  -n cert-manager \
  --set groupName='<YOUR_UNIQUE_GROUP_NAME>' \
  deploy/cert-manager-webhook-coredns/
```

## Running the test suite

All DNS providers **must** run the DNS01 provider conformance testing suite,
else they will have undetermined behaviour when used with cert-manager.

**It is essential that you configure and run the test suite when creating a
DNS01 webhook.**

An example Go test file has been provided in [main_test.go](https://github.com/cert-manager/webhook-example/blob/master/main_test.go).

Before you can run the test suite, you need to duplicate the `.sample` files in `testdata/coredns-solver/` and update the configuration with the appropriate ETCD credentials.

You can run the test suite with:

```bash
$ TEST_ZONE_NAME=example.com. make test
```

The example file has a number of areas you must fill in and replace with your own options in order for tests to pass.

## Acknowledgments

This work is based on: https://github.com/cert-manager/webhook-example. 

Another open-source project https://github.com/ii/cert-manager-webhook-coredns-etcd licenced using Apache2 preceeds this work.
This work is not per-se a fork of it but is inspired by it.

Also, the more mature project [cert-manager-webhook-ovh](https://github.com/baarde/cert-manager-webhook-ovh/tree/main) served as an example.

Finally, the ExternalDNS [CoreDNS](https://github.com/kubernetes-sigs/external-dns/blob/master/provider/coredns/coredns.go) plugin has been a great help for the realisation of this work.