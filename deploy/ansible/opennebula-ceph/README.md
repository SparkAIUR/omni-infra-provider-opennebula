# OpenNebula Ceph Ansible Playbook

This Ansible workflow provisions the Ceph backend required by an OpenNebula environment and prepares the datastore metadata expected by the OpenNebula CSI storage driver.

It is a public, workstation-driven alternative to the repo-local shell workflow under `.assh/scripts/setup-production-ceph-storage.sh`. Both paths are intended to produce the same resulting infrastructure contract:

- Ceph RBD pools: `one-images`, `one-system`, `one-csi`
- CephFS pools: `one-cephfs-meta`, `one-cephfs-data`
- CephFS filesystem: `one-cephfs`
- CephFS subvolume group: `csi`
- OpenNebula datastores:
  - `ceph-images`
  - `ceph-system`
  - `one-csi`
  - `one-csi-cephfs`
- Ceph users:
  - `client.opennebula`
  - `client.opennebula-csi`
  - `client.opennebula-csi-node`

## What It Does

The playbook provisions:

- Ceph bootstrap and host enrollment
- OSD device registration for the explicitly listed disks only
- RBD pools and CephFS
- Ceph auth users for OpenNebula and CSI
- OpenNebula Ceph client keyrings and libvirt secrets on the configured client hosts
- OpenNebula datastores on the frontend using the Ceph metadata contract expected by the CSI driver

It does not:

- migrate existing workloads
- switch current Talos root disks to Ceph
- apply Kubernetes CSI manifests
- remove or resize existing Ceph or OpenNebula objects

## Layout

- `site.yml`: main entrypoint
- `inventory/production.example.yml`: example inventory
- `group_vars/all.yml`: public variable contract
- `roles/ceph_bootstrap`: initial Ceph bootstrap and MON/MGR setup
- `roles/ceph_cluster`: host enrollment, OSDs, pools, CephFS, MDS
- `roles/ceph_auth`: OpenNebula and CSI Ceph users, local artifacts
- `roles/opennebula_ceph_clients`: keyrings, raw key placement, libvirt secret setup
- `roles/opennebula_datastores`: datastore rendering and creation
- `templates/`: datastore and libvirt templates

## Prerequisites

- Ansible on the operator workstation
- SSH access to the Ceph and OpenNebula hosts
- `cephadm` already present on the bootstrap host unless you intentionally extend this workflow
- OpenNebula CLI available on the configured frontend
- `virsh` available on each OpenNebula client host

## Quick Start

Copy and edit the example inventory:

```bash
cp deploy/ansible/opennebula-ceph/inventory/production.example.yml \
  deploy/ansible/opennebula-ceph/inventory/production.yml
```

Run syntax validation:

```bash
ansible-playbook \
  -i deploy/ansible/opennebula-ceph/inventory/production.yml \
  deploy/ansible/opennebula-ceph/site.yml \
  --syntax-check
```

Run a dry run:

```bash
ansible-playbook \
  -i deploy/ansible/opennebula-ceph/inventory/production.yml \
  deploy/ansible/opennebula-ceph/site.yml \
  --check --diff
```

Run the full apply:

```bash
ansible-playbook \
  -i deploy/ansible/opennebula-ceph/inventory/production.yml \
  deploy/ansible/opennebula-ceph/site.yml
```

Run a single phase by tag:

```bash
ansible-playbook \
  -i deploy/ansible/opennebula-ceph/inventory/production.yml \
  deploy/ansible/opennebula-ceph/site.yml \
  --tags ceph-auth
```

Supported tags:

- `inventory`
- `ceph-bootstrap`
- `ceph-cluster`
- `ceph-auth`
- `opennebula-clients`
- `opennebula-datastores`
- `postcheck`

## Artifacts

The playbook stores operator-only Ceph artifacts under:

```text
deploy/ansible/opennebula-ceph/artifacts/
```

This directory is intended for locally generated keyrings and rendered datastore templates. Do not commit its contents.

## Safety Model

- The playbook is additive only.
- It fails if the configured Talos template no longer matches the expected local root-disk scheduling marker.
- It only enrolls the hosts and OSD devices explicitly declared in inventory.
- It does not overwrite an unexpected libvirt secret UUID.
- It does not recreate existing pools, filesystems, or datastores.
