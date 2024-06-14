<p align="center">
  <img src="https://raw.githubusercontent.com/cert-manager/cert-manager/d53c0b9270f8cd90d908460d69502694e1838f5f/logo/logo-small.png" height="256" width="256" alt="cert-manager project logo" />
</p>

# Cert-Manager webhook for CoreDNS

Cert-Manager `dns01` webhook for CoreDNS using ETCD plugin.  
See https://cert-manager.io/docs/configuration/acme/dns01/webhook/ for more information.


## Usage

1. Create a secret containing your etcd credentials in the same namespace than the webhook

```sh 
kubectl create secret generic etcd-credentials \
  --from-literal=etcd-username='ETCD-USERNAME' \
  --from-literal=etcd-password='ETCD-PASSWORD' \
  -n cert-manager
```

2. Create RBAC configuration to access secret

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: cert-manager-webhook-coredns:secret-reader
  namespace: cert-manager
rules:
- apiGroups: [""]
  resources: ["secrets"]
  resourceNames: ["etcd-credentials"]
  verbs: ["get", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: cert-manager-webhook-coredns:secret-reader
  namespace: cert-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: cert-manager-webhook-coredns:secret-reader
subjects:
- apiGroup: ""
  kind: ServiceAccount
  name: cert-manager-webhook-coredns
```

3. Create a `ClusterIssuer` or `Issuer`

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: nextheader-acme
spec:
  acme:
    email: admin@nextheader.dev
    server: https://acme-v02.api.letsencrypt.org/directory
    privateKeySecretRef:
      name: nextheader-acme
    solvers:
    - dns01:
        webhook:
          groupName: acme.nextheader.dev
          solverName: coredns-solver
          config:
            coreDNSPrefix: /skydns
            etcdEndpoints: "http://[2a06:de00:50:1:ff00::11]:2379"
            etcdUsernameRef:
              name: etcd-credentials
              key: etcd-username        
            etcdPasswordRef: 
              name: etcd-credentials
              key: etcd-password
```

4. Finally, install the Cert-Manager webhook for CoreDNS. Choose a unique group name to identify your company or organization (for example `acme.mycompany.example`). In this example it is installed in the `cert-manager` namespace.

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