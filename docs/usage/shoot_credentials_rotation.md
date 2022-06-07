# Credentials Rotation For Shoot Clusters

There are a lot of different credentials for `Shoot`s to make sure that the various components can communicate with each other, and to make sure it is usable and operable.

This page explains how the varieties of credentials can be rotated so that the cluster can be considered secure.

## User-Provided Credentials

### Cloud Provider Keys

End-users must provide credentials such that Gardener and Kubernetes controllers can communicate with the respective cloud provider APIs in order to perform infrastructure operations.
For example, Gardener uses them to setup and maintain the networks, security groups, subnets, etc., while the [cloud-controller-manager](https://kubernetes.io/docs/concepts/architecture/cloud-controller/) uses them to reconcile load balancers and routes, and the [CSI controller](https://kubernetes-csi.github.io/docs/) uses them to reconcile volumes and disks.

Depending on the cloud provider, the required [data keys of the `Secret` differ](https://github.com/gardener/gardener/blob/master/example/70-secret-provider.yaml).
Please consult the documentation of the respective provider extension documentation to get to know the concrete data keys (e.g., [this document for AWS](https://github.com/gardener/gardener-extension-provider-aws/blob/master/docs/usage-as-end-user.md#provider-secret-data)).

**It is the responsibility of the end-user to regularly rotate those credentials.**
The following steps are required to perform the rotation:

1. Update the data in the `Secret` with new credentials.
2. ⚠️ Wait until all `Shoot`s using the `Secret` are reconciled before you disable the old credentials in your cloud provider account! Otherwise, the `Shoot`s will no longer work as expected. Check out [this document](shoot_operations.md#immediate-reconciliation) to learn how to trigger a reconciliation of your `Shoot`s.
3. After all `Shoot`s using the `Secret` were reconciled, you can go ahead and deactivate the old credentials in your provider account account.

## Gardener-Provided Credentials

Below credentials are generated by Gardener when shoot clusters are being created.
Those include

- kubeconfig (if enabled)
- certificate authorities (and related server and client certificates)
- observability passwords for Grafana
- SSH key pair for worker nodes
- ETCD encryption key
- `ServiceAccount` token signing key
- ...

**🚨 There is no auto-rotation of those credentials, and it is the responsibility of the end-user to regularly rotate them.**

While it is possible to rotate them one by one, there is also a convenient method to combine the rotation of all of those credentials.
The rotation happens in two phases since it might be required to update some API clients (e.g., when CAs are rotated).
In order to start the rotation (first phase), you have to annotate the shoot with the `rotate-credentials-start` operation:

```bash
kubectl -n <shoot-namespace> annotate shoot <shoot-name> gardener.cloud/operation=rotate-credentials-start
```

> You can check the `.status.credentials.rotation` field in the `Shoot` to see when the rotation was last initiated and last completed.


Kindly consider the detailed descriptions below to learn how the rotation is performed and what your responsibilities are.
Please note that all respective individual actions apply for this combined rotation as well (e.g., worker nodes are rolled out in the first phase).

You can complete the rotation (second phase) by annotating the shoot with the `rotate-credentials-complete` operation:

```bash
kubectl -n <shoot-namespace> annotate shoot <shoot-name> gardener.cloud/operation=rotate-credentials-complete
```

### Kubeconfig

If the `.spec.kubernetes.enableStaticTokenKubeconfig` field is set to `true` (default) then Gardener generates a `kubeconfig` with `cluster-admin` privileges for the `Shoot`s containing credentials for communication with the `kube-apiserver` (see [this document](shoot_access.md#static-token-kubeconfig) for more information).

This `Secret` is stored with name `<shoot-name>.kubeconfig` in the project namespace in the garden cluster and has multiple data keys:

- `kubeconfig`: the completed kubeconfig
- `token`: token for `system:cluster-admin` user
- `username`/`password`: basic auth credentials (if enabled via `Shoot.spec.kubernetes.kubeAPIServer.enableBasicAuthentication`)
- `ca.crt`: the CA bundle for establishing trust to the API server (same as in the [Cluster CA bundle secret](#cluster-certificate-authority-bundle))

> `Shoots` created with Gardener <= 0.28 used to have a `kubeconfig` based on a client certificate instead of a static token. With the first kubeconfig rotation, such clusters will get a static token as well.
>
> ⚠️ This does not invalidate the old client certificate. In order to do this, you should perform a rotation of the CAs (see section below).

**It is the responsibility of the end-user to regularly rotate those credentials (or disable this `kubeconfig` entirely).**
In order to rotate the `token` in this `kubeconfig`, annotate the `Shoot` with `gardener.cloud/operation=rotate-kubeconfig-credentials`.
This operation is not allowed for `Shoot`s that are already marked for deletion.
Please note that only the token (and basic auth password, if enabled) are exchanged.
The CA certificate remains the same (see section below for information about the rotation).

```bash
kubectl -n <shoot-namespace> annotate shoot <shoot-name> gardener.cloud/operation=rotate-kubeconfig-credentials
```

> You can check the `.status.credentials.rotation.kubeconfig` field in the `Shoot` to see when the rotation was last initiated and last completed.

### Certificate Authorities

Gardener generates several certificate authorities (CAs) to ensure secured communication between the various components and actors.
Most of those CAs are used for internal communication (e.g., `kube-apiserver` talks to etcd, `vpn-shoot` talks to the `vpn-seed-server`, `kubelet` talks to `kube-apiserver` etc.).
However, there is also the "cluster CA" which is part of all `kubeconfig`s and used to sign the server certificate exposed by the `kube-apiserver`.

Gardener populates a `Secret` with name `<shoot-name>.ca-cluster` in the project namespace in the garden cluster which contains the following data keys:

- `ca.crt`: the CA bundle of the cluster

This bundle contains one or multiple CAs which are used for signing serving certificates of the `Shoot`'s API server.
Hence, the certificates contained in this `Secret` can be used to verify the API server's identity when communicating with its public endpoint (e.g. as `certificate-authority-data` in a `kubeconfig`).
This is the same certificate that is also contained in the `kubeconfig`'s `certificate-authority-data` field.

> `Shoot`s created with Gardener >= v1.45 have a dedicated client CA which verifies the legitimacy of client certificates. For older `Shoot`s, the client CA is equal to the cluster CA. With the first CA rotation, such clusters will get a dedicated client CA as well.

All of the certificates are valid for 10 years.
Since it requires adaptation for the consumers of the `Shoot`, there is no automatic rotation and **it is the responsibility of the end-user to regularly rotate the CA certificates.**

> Note that the CA rotation can only be triggered if the `ShootCARotation` feature gate is enabled.

The rotation happens in three stages (see also [GEP-18](../proposals/18-shoot-CA-rotation.md) for the full details):

- In stage one, new CAs are created and added to the bundle (together with the old CAs). Client certificates are re-issued immediately.
- In stage two, end-users update all cluster API clients that communicate with the control plane.
- In stage three, the old CAs are dropped from the bundle and server certificate are re-issued.

Technically, the `Preparing` phase indicates stage one.
Once it is completed, the `Prepared` phase indicates readiness for stage two.
The `Completing` phase indicates stage three, and the `Completed` phase states that the rotation process has finished.

> You can check the `.status.credentials.rotation.certificateAuthorities` field in the `Shoot` to see when the rotation was last initiated, last completed, and in which phase it currently is.

In order to start the rotation (stage one), you have to annotate the shoot with the `rotate-ca-start` operation:

```bash
kubectl -n <shoot-namespace> annotate shoot <shoot-name> gardener.cloud/operation=rotate-ca-start
```

This will trigger a `Shoot` reconciliation and performs stage one.
After it is completed, the `.status.credentials.rotation.certificateAuthorities.phase` is set to `Prepared`.

Now you must update all API clients outside the cluster (such as the `kubeconfig`s on developer machines) to use the newly issued CA bundle in the `<shoot-name>.ca-cluster` `Secret`.
Please also note that client certificates must be re-issued now.

After updating all API clients, you can complete the rotation by annotating the shoot with the `rotate-ca-complete` operation:

```bash
kubectl -n <shoot-namespace> annotate shoot <shoot-name> gardener.cloud/operation=rotate-ca-complete
```

This will trigger another `Shoot` reconciliation and performs stage three.
After it is completed, the `.status.credentials.rotation.certificateAuthorities.phase` is set to `Completed`.
You could update your API clients again and drop the old CA from their bundle.

> Note that the CA rotation also rotates all internal CAs and signed certificates.
Hence, most of the components need to be restarted (including etcd and `kube-apiserver`).
>
> ⚠️ In stage one, all worker nodes of the `Shoot` will be rolled out to ensure that the `Pod`s as well as the `kubelet`s get the updated credentials as well.

### Observability Password(s) For Grafana

For `Shoot`s with `.spec.purpose!=testing`, Gardener deploys an observability stack with Prometheus for monitoring, Alertmanager for alerting (optional), Loki for logging, and Grafana for visualization.
The Grafana instance is exposed via `Ingress` and accessible for end-users via basic authentication credentials generated and managed by Gardener.

Those credentials are stored in a `Secret` with name `<shoot-name>.monitoring` in the project namespace in the garden cluster and has multiple data keys:

- `username`: the user name
- `password`: the password
- `basic_auth.csv`: the user name and password in CSV format
- `auth`: the user name with SHA-1 representation of the password

**It is the responsibility of the end-user to regularly rotate those credentials.**
In order to rotate the `password`, annotate the `Shoot` with `gardener.cloud/operation=rotate-observability-credentials`.
This operation is not allowed for `Shoot`s that are already marked for deletion.

```bash
kubectl -n <shoot-namespace> annotate shoot <shoot-name> gardener.cloud/operation=rotate-observability-credentials
```

> You can check the `.status.credentials.rotation.observability` field in the `Shoot` to see when the rotation was last initiated and last completed.

#### Operators

Gardener operators have separate credentials to access their own Grafana instance or Prometheus, Alertmanager, Loki directly.
These credentials are only stored in the shoot namespace in the seed cluster and can be retrieved as follows:

```bash
kubectl -n shoot--<project>--<name> get secret -l name=observability-ingress,managed-by=secrets-manager,manager-identity=gardenlet
```

These credentials are only valid for `30d` and get automatically rotated with the next `Shoot` reconciliation when 80% of the validity approaches or when there are less than `10d` until expiration.
There is no way to trigger the rotation manually.

### SSH Key Pair For Worker Nodes

Gardener generates an SSH key pair whose public key is propagated to all worker nodes of the `Shoot`.
The private key can be used to establish an SSH connection to the workers for troubleshooting purposes.
It is recommended to use [`gardenctl-v2`](https://github.com/gardener/gardenctl-v2/) and its `gardenctl ssh` command since it is required to first open up the security groups and create a bastion VM (no direct SSH access to the worker nodes is possible).

The private key is stored in a `Secret` with name `<shoot-name>.ssh-keypair` in the project namespace in the garden cluster and has multiple data keys:

- `id_rsa`: the private key
- `id_rsa.pub`: the public key for SSH

In order to rotate the keys, annotate the `Shoot` with `gardener.cloud/operation=rotate-ssh-keypair`.
This will propagate a new key to all worker nodes while keeping the old key active and valid as well (it will only be invalidated/removed with the next rotation).

```bash
kubectl -n <shoot-namespace> annotate shoot <shoot-name> gardener.cloud/operation=rotate-ssh-keypair
```

> You can check the `.status.credentials.rotation.sshKeypair` field in the `Shoot` to see when the rotation was last initiated or last completed.

The old key is stored in a `Secret` with name `<shoot-name>.ssh-keypair.old` in the project namespace in the garden cluster and has the same data keys as the regular `Secret`.

> Note that the SSH keypairs for shoot clusters are rotated automatically during maintenance time window when the `RotateSSHKeypairOnMaintenance` feature gate is enabled.
However, this feature gate is deprecated, turned off by default and will be removed in a future version of Gardener.

### ETCD Encryption Key

This key is used to encrypt the data of `Secret` resources inside etcd (see [upstream Kubernetes documentation](https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/)).

The encryption key has no expiration date.
There is no automatic rotation and **it is the responsibility of the end-user to regularly rotate the encryption key.**

The rotation happens in three stages:

- In stage one, a new encryption key is created and added to the bundle (together with the old encryption key).
- In stage two, all `Secret`s in the cluster are rewritten by the `kube-apiserver` so that they become encrypted with the new encryption key.
- In stage three, the old encryption is dropped from the bundle.

Technically, the `Preparing` phase indicates the stages one and two.
Once it is completed, the `Prepared` phase indicates readiness for stage three.
The `Completing` phase indicates stage three, and the `Completed` phase states that the rotation process has finished.

> You can check the `.status.credentials.rotation.etcdEncryptionKey` field in the `Shoot` to see when the rotation was last initiated, last completed, and in which phase it currently is.

In order to start the rotation (stage one), you have to annotate the shoot with the `rotate-etcd-encryption-key-start` operation:

```bash
kubectl -n <shoot-namespace> annotate shoot <shoot-name> gardener.cloud/operation=rotate-etcd-encryption-key-start
```

This will trigger a `Shoot` reconciliation and performs the stages one and two.
After it is completed, the `.status.credentials.rotation.etcdEncryptionKey.phase` is set to `Prepared`.
Now you can complete the rotation by annotating the shoot with the `rotate-etcd-encryption-key-complete` operation:

```bash
kubectl -n <shoot-namespace> annotate shoot <shoot-name> gardener.cloud/operation=rotate-etcd-encryption-key-complete
```

This will trigger another `Shoot` reconciliation and performs stage three.
After it is completed, the `.status.credentials.rotation.etcdEncryptionKey.phase` is set to `Completed`.

### `ServiceAccount` Token Signing Key

Gardener generates a key which is used to sign the tokens for [`ServiceAccount`s](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/).
Those tokens are typically used by workload `Pod`s running inside the cluster in order to authenticate themselves with the `kube-apiserver`.
This also includes system components running in the `kube-system` namespace.

The token signing key has no expiration date.
Since it might require adaptation for the consumers of the `Shoot`, there is no automatic rotation and **it is the responsibility of the end-user to regularly rotate the signing key.**

> Note that the signing key rotation can only be triggered if the `ShootSARotation` feature gate is enabled.

The rotation happens in three stages, similar to how the [CA certificates](#certificate-authorities) are rotated:

- In stage one, a new signing key is created and added to the bundle (together with the old signing key).
- In stage two, end-users update all out-of-cluster API clients that communicate with the control plane via `ServiceAccount` tokens.
- In stage three, the old signing key is dropped from the bundle.

Technically, the `Preparing` phase indicates stage one.
Once it is completed, the `Prepared` phase indicates readiness for stage two.
The `Completing` phase indicates stage three, and the `Completed` phase states that the rotation process has finished.

> You can check the `.status.credentials.rotation.serviceAccountKey` field in the `Shoot` to see when the rotation was last initiated, last completed, and in which phase it currently is.

In order to start the rotation (stage one), you have to annotate the shoot with the `rotate-serviceaccount-key-start` operation:

```bash
kubectl -n <shoot-namespace> annotate shoot <shoot-name> gardener.cloud/operation=rotate-serviceaccount-key-start
```

This will trigger a `Shoot` reconciliation and performs stage one.
After it is completed, the `.status.credentials.rotation.serviceAccountKey.phase` is set to `Prepared`.

Now you must update all API clients outside the cluster using a `ServiceAccount` token (such as the `kubeconfig`s on developer machines) to use a token issued by the new signing key.
Gardener already generates new static token secrets for all `ServiceAccount`s in the cluster.
However, if you need to create it manually, you can check out [this document](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#manually-create-a-service-account-api-token) for instructions.

After updating all API clients, you can complete the rotation by annotating the shoot with the `rotate-serviceaccount-key-complete` operation:

```bash
kubectl -n <shoot-namespace> annotate shoot <shoot-name> gardener.cloud/operation=rotate-serviceaccount-key-complete
```

This will trigger another `Shoot` reconciliation and performs stage three.
After it is completed, the `.status.credentials.rotation.serviceAccountKey.phase` is set to `Completed`.

> ⚠️ In stage one, all worker nodes of the `Shoot` will be rolled out to ensure that the `Pod`s use a new token.

### OpenVPN TLS Auth Keys

This key is used to ensure encrypted communication for the VPN connection between the control plane in the seed cluster and the shoot cluster.
It is currently **not** rotated automatically and there is no way to trigger it manually.