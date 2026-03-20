# Gen2 Staging Stack Creation

This document captures the current working shape of the Rackspace Spot Gen2 staging lab used for `omni-infra-provider-opennebula`.

It is written as an operator rebuild guide for the case where the Spot nodes are terminated and the lab must be recreated from scratch.

## Topology

Frontend and services:

- `lab-fe-01`
  - public: `50.56.158.179`
  - private: `10.0.2.224`
  - roles:
    - OpenNebula frontend
    - OpenNebula hypervisor
    - Ceph MON/MGR/MDS/OSD
    - Omni
    - Dex
    - `omni-infra-provider-opennebula`
    - nginx/TLS
    - guest NAT and `onebr1` gateway

Hypervisors and Ceph nodes:

- `lab-hv-01`
  - public: `50.56.158.126`
  - private: `10.0.1.167`
- `lab-hv-02`
  - public: `50.56.159.19`
  - private: `10.0.3.200`

All three nodes are Ubuntu `24.04.3`.

## Disk layout

Use root (`/`) for:

- OpenNebula frontend packages and runtime state
- local OpenNebula datastores
- Omni, Dex, nginx, tooling

Use `/dev/vdb` for Ceph:

1. Unmount `/mnt`
2. Remove `/mnt` from `/etc/fstab`
3. Wipe `/dev/vdb`
4. Hand `/dev/vdb` to Ceph as one raw OSD per node

Leave `/dev/vdc` as swap unless there is a strong reason to change it.

## Networking

### Underlay

- Use the private Rackspace addresses for east-west traffic:
  - `10.0.2.224`
  - `10.0.1.167`
  - `10.0.3.200`
- Use the private network for:
  - Ceph public traffic
  - Ceph cluster traffic
  - OpenNebula host communication
  - SSH between nodes
  - VXLAN guest overlay traffic

Validate reachability early:

```bash
ping -c 3 10.0.2.224
ping -c 3 10.0.1.167
ping -c 3 10.0.3.200
```

### Guest overlay

The Talos guests run on a multi-host `onebr1` overlay.

- bridge: `onebr1`
- guest subnet: `172.22.0.0/24`
- gateway on frontend: `172.22.0.1/24`
- VXLAN VNI: `422`
- guest MTU: `1450`

The frontend owns the guest gateway and NAT.

Requirements on `lab-fe-01`:

- enable IPv4 forwarding
- NAT `172.22.0.0/24` to the public uplink
- apply TCP MSS clamping on guest egress so TLS and gRPC flows do not blackhole across the VXLAN MTU boundary

Things to verify:

```bash
ip addr show onebr1
ip link show
bridge link
sysctl net.ipv4.ip_forward
nft list ruleset
```

### Firewall expectations

On the frontend, allow public ingress for:

- `22`
- `80`
- `443`
- `8090`
- `8100`

On the secondary hypervisors, only expose `22` publicly.

Allow private east-west traffic required for:

- Ceph
- libvirt/qemu/OpenNebula host operations
- VXLAN
- SSH

## Ceph

This staging stack uses a single `cephadm` cluster on the three private IPs.

### Placement

- MONs: all three hosts
- MGRs: frontend active, one standby
- OSDs: all three hosts on raw `/dev/vdb`
- MDS: two daemons for CephFS

### Pools and filesystems

Create:

- RBD pools:
  - `one-images`
  - `one-system`
  - `one-csi`
- CephFS:
  - metadata pool: `one-cephfs-meta`
  - data pool: `one-cephfs-data`
  - filesystem: `one-cephfs`
  - subvolume group: `csi`

Use replication:

- `size=3`
- `min_size=2`

### Validation

```bash
ceph -s
ceph osd tree
ceph osd pool ls detail
ceph fs status
ceph fs subvolumegroup ls one-cephfs
```

The monitors are expected to be reachable by private IP from the future CSI pods.

## OpenNebula

### Endpoint and credentials

- public UI/API domain: `https://on.lab.sprkinfra.com`
- provider XML-RPC endpoint: `http://127.0.0.1:2633/RPC2`
- operator credentials:
  - user: `oneadmin`
  - password: `fxSYsDn6EP7HX30QekcBpjpnnyt5xECh`

### Host registration

Register all three nodes in one cluster:

- `lab-fe-01`
- `lab-hv-01`
- `lab-hv-02`

Current lab caveat:

- `/dev/kvm` is absent on these Gen2 Spot hosts
- use `qemu` for the staging lab
- the provider now supports `opennebula.hypervisor: auto|kvm|qemu`
- `auto` should prefer `kvm` when production hosts expose it, then fall back to `qemu`

### Datastores

Current intended datastore set:

- `default`
  - local image datastore
  - used for lightweight root-disk scenarios
- `one-csi-local`
  - local datastore for CSI local-path testing
- `ceph-images`
  - Ceph-backed image datastore
- `ceph-system`
  - Ceph-backed system datastore
- `one-csi`
  - Ceph RBD datastore for CSI
- `one-csi-cephfs`
  - `FILE` datastore carrying CephFS metadata for RWX

Useful commands:

```bash
onedatastore list
onedatastore show <id>
oneimage list
onetemplate list
onehost list
onehost show <id>
```

### Networks

Create:

- `talos-stage-auto`
  - bridge: `onebr1`
  - auto leases from `172.22.0.100+`
  - `GUEST_MTU="1450"`
- `talos-stage-manual`
  - bridge: `onebr1`
  - manual/MAC-oriented path
  - `GUEST_MTU="1450"`

### Talos template

Base template name:

- `talos-omni-base`

Important properties:

- no hardcoded staging IPs or credentials
- hostname injected via `SET_HOSTNAME`
- `NIC.MODEL="virtio"`
- graphics disabled for Omni machines
- hypervisor now resolved by provider config

## FireEdge and Sunstone login troubleshooting

The most important auth gotcha in this lab was FireEdge login returning HTTP 500 after rebuilding the frontend.

### Symptom

- browser login POST to `/fireedge/api/auth/` returns `500`
- browser console shows:
  - `POST https://on.lab.sprkinfra.com/fireedge/api/auth/ 500`
- `fireedge.log` shows:
  - `JWTError: Signature verification failed`
- `oned.log` shows:
  - `server_cipher/authenticate`
  - `bad decrypt`

### Root cause

The OpenNebula `serveradmin` database password and the plain secrets in the auth files were not aligned.

The server-cipher auth flow is non-obvious:

- the DB password for `serveradmin` must be `sha256(plain_secret)`
- the auth files must contain the plain secret:
  - `serveradmin:<plain_secret>`

Relevant files and commands on the frontend:

- `/var/lib/one/.one/sunstone_auth`
- `/var/lib/one/.one/oneflow_auth`
- `/var/lib/one/.one/onegate_auth`
- `/root/.opennebula-serveradmin-secret`

### Recovery procedure

1. Generate a fresh secret.
2. Set the DB password using SHA256:

```bash
SECRET="$(openssl rand -hex 24)"
sudo -u oneadmin -H oneuser passwd serveradmin --sha256 "$SECRET"
```

3. Write the plain secret to the auth files:

```bash
printf 'serveradmin:%s\n' "$SECRET" >/var/lib/one/.one/sunstone_auth
printf 'serveradmin:%s\n' "$SECRET" >/var/lib/one/.one/oneflow_auth
printf 'serveradmin:%s\n' "$SECRET" >/var/lib/one/.one/onegate_auth
chown oneadmin:oneadmin /var/lib/one/.one/sunstone_auth /var/lib/one/.one/oneflow_auth /var/lib/one/.one/onegate_auth
chmod 600 /var/lib/one/.one/sunstone_auth /var/lib/one/.one/oneflow_auth /var/lib/one/.one/onegate_auth
printf '%s\n' "$SECRET" >/root/.opennebula-serveradmin-secret
chmod 600 /root/.opennebula-serveradmin-secret
```

4. Restart services:

```bash
systemctl restart opennebula opennebula-fireedge opennebula-flow opennebula-gate
```

5. Validate:

```bash
curl -sk -X POST https://on.lab.sprkinfra.com/fireedge/api/auth/ \
  -H 'Content-Type: application/json' \
  --data '{"user":"oneadmin","password":"fxSYsDn6EP7HX30QekcBpjpnnyt5xECh"}'
```

Expected result:

- HTTP `200`
- clean login from a fresh browser session

If the UI still fails after the server-side fix, clear cookies and local storage for `on.lab.sprkinfra.com`.

## Omni

### Domains

- UI/API: `https://omni.on.lab.sprkinfra.com`
- machine API: `https://omni.on.lab.sprkinfra.com:8090/`
- workload proxy: `https://omni.on.lab.sprkinfra.com:8100/`

### Runtime shape

- Omni version: `1.6.0`
- gRPC tunnel enabled for machine connectivity
- provider registration name: `opennebula`

The local admin environment on the frontend is:

- `/root/.config/omni/admin.env`

Useful pattern:

```bash
set -a
. /root/.config/omni/admin.env
set +a
omnictl --insecure-skip-tls-verify get machineclass -o table
```

### Current machine classes

The lab currently uses classes in the `opennebula-stage-*` family. They should:

- use provider `opennebula`
- set `grpctunnel: 1`
- use the OpenNebula provider data contract
- keep VM name aligned with Talos hostname by using `cluster-role-sequence`

## Dex

- domain: `https://dex.on.lab.sprkinfra.com`
- email/user: `admin@sprkinfra.com`
- username: `sparkaiur`
- password: `ZLe4Y50LzDUDbA7JvaGdeYqlr2EDAX7W`

## Provider deployment

### Service layout

Current frontend service:

- systemd unit: `/etc/systemd/system/omni-infra-provider-opennebula.service`
- working directory: `/opt/omni-provider-opennebula`
- config file: `/opt/omni-provider-opennebula/config.yaml`
- env file: `/opt/omni-provider-opennebula/provider.env`

### Important current config characteristics

- `opennebula.endpoint: http://127.0.0.1:2633/RPC2`
- `defaults.hostnameStrategy: cluster-role-sequence`
- `features.allowExplicitResources: true`
- `storagePolicies.defaultDatastore: default`
- `imageManagement.importOnMiss: true`
- `imageManagement.requireChecksum: true`
- Talos image artifact template uses the OpenNebula `qcow2` variant

### Current image artifact pattern

The staging lab should use:

```text
https://factory.talos.dev/image/<schematic-id>/<talos-version-no-v>/opennebula-amd64.qcow2
```

Do not fall back to the older `.raw.zst` staging helper pattern when rebuilding this lab.

## DNS and TLS

Cloudflare-managed domains:

- `on.lab.sprkinfra.com`
- `omni.on.lab.sprkinfra.com`
- `dex.on.lab.sprkinfra.com`

Keep them as DNS-only records, not proxied.

Certificates should cover all three names with DNS-01 against Cloudflare.

Store the Cloudflare token on the frontend in a root-only file and do not commit it into the repo.

## `hplcsi` recreation guidance

Target shape:

- cluster: `hplcsi`
- Talos: `1.12.5`
- Kubernetes: `1.34.5`
- `1` control plane
- `1` worker
- root disks: `20Gi`
- preferred non-Ceph datastore: `one-csi-local`

The important staging lesson is that the handoff cluster should keep its root disks off Ceph so the CSI team has more headroom for their own experiments.

Do not assume `default` is always valid for this.

In the current lab:

- `default` is an `IMAGE_DS` with `TM_MAD=local`
- the cluster also has a Ceph system datastore (`ceph-system`)
- OpenNebula can reject that combination with:
  - `Image Datastore does not support transfer mode: ceph`

If that happens, use `one-csi-local` instead. It is still non-Ceph from the CSI team’s perspective and is compatible with the current lab’s transfer path.

Verify after recreate:

- VM name matches Talos hostname
- datastore is `default`
- gRPC tunnel kernel arg is present
- nodes converge without manual patching

## Validation checklist

OpenNebula:

- `onehost list`
- `onedatastore list`
- `onevnet list`
- `onetemplate list`
- fresh Sunstone login works

Ceph:

- `ceph -s`
- `ceph osd tree`
- `ceph fs status`

Omni:

- `omnictl --insecure-skip-tls-verify get machineclass -o table`
- `omnictl --insecure-skip-tls-verify get machinerequest -o table`
- `omnictl --insecure-skip-tls-verify get cluster -o table`
- `omnictl --insecure-skip-tls-verify cluster status <cluster>`

Provider:

- `systemctl status omni-infra-provider-opennebula`
- `journalctl -u omni-infra-provider-opennebula -n 200 --no-pager`

## Recovery checklist after Spot replacement

1. Bring up three Gen2 nodes and assign the expected hostnames.
2. Re-establish private east-west connectivity.
3. Rebuild `onebr1` and the VXLAN guest overlay.
4. Reapply NAT and MSS clamping on the frontend.
5. Recreate Ceph and validate pool/filesystem health.
6. Reinstall OpenNebula frontend and register all three hosts.
7. Recreate datastores, VNETs, and `talos-omni-base`.
8. Reinstall Dex, Omni, and the provider.
9. Re-register infra provider `opennebula`.
10. Repoint Cloudflare DNS and issue certificates.
11. Validate FireEdge login before doing any Omni cluster work.
12. Recreate `hplcsi` and hand the kubeconfig and cluster notes to the CSI team.
