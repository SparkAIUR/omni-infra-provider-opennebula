from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any, Literal

import yaml
from pydantic import BaseModel, ConfigDict, Field, model_validator


class Model(BaseModel):
    model_config = ConfigDict(extra="forbid")


class LabConfig(Model):
    name: str
    stateDir: str
    workspaceDir: str
    logDir: str


class ServerClassConstraints(Model):
    generation: int
    minVcpu: int
    minMemoryGiB: int


class RackspaceNodeRequest(Model):
    name: str
    count: int = 1
    serverClassConstraints: ServerClassConstraints


class RackspaceHypervisorRequest(Model):
    name: str
    generation: int
    minVcpu: int
    minMemoryGiB: int


class RackspaceConfig(Model):
    enabled: bool = False
    rsspotConfigPath: str | None = None
    profile: str | None = None
    region: str | None = None
    sshPublicKeyPath: str | None = None
    nodeRequests: dict[str, Any] = Field(default_factory=dict)


class NodeConfig(Model):
    hostname: str
    publicIP: str | None = None
    privateIP: str | None = None
    sshUser: str = "root"
    sshPort: int = 22
    roles: list[str] = Field(default_factory=list)


class NodesConfig(Model):
    frontend: NodeConfig
    hypervisors: list[NodeConfig]

    def all_nodes(self) -> list[NodeConfig]:
        return [self.frontend, *self.hypervisors]

    def by_hostname(self, hostname: str) -> NodeConfig:
        for node in self.all_nodes():
            if node.hostname == hostname:
                return node
        raise KeyError(hostname)


class NetworkingConfig(Model):
    underlayInterface: str
    guestBridge: str
    guestVXLANDevice: str
    guestSubnet: str
    guestGatewayCIDR: str
    guestLeaseStart: str
    guestMTU: int
    vxlanVni: int
    natCIDR: str
    openPublicPortsFrontend: list[int]
    openPublicPortsHypervisors: list[int]


class CephFSConfig(Model):
    metadataPool: str
    dataPool: str
    fsName: str
    subvolumeGroup: str


class CephPoolsConfig(Model):
    rbd: list[str]
    cephfs: CephFSConfig


class CephReplicationConfig(Model):
    size: int
    minSize: int


class CephConfig(Model):
    enabled: bool = True
    versionChannel: str
    rawOsdDevice: str
    fsid: str = ""
    publicNetworkCIDR: str
    clusterNetworkCIDR: str
    mons: list[str]
    mgrActive: str
    mgrStandby: str
    mdsCount: int
    pools: CephPoolsConfig
    replication: CephReplicationConfig


class TemplateConfig(Model):
    name: str
    hostnameMode: str
    nicModel: str
    graphicsEnabled: bool


class DatastoreConfig(Model):
    name: str
    kind: str
    backend: str
    pool: str | None = None
    fsName: str | None = None


class OpenNebulaNetworkConfig(Model):
    name: str
    mode: Literal["auto", "manual"]
    bridge: str
    mtu: int
    networkCIDR: str | None = None
    leaseStart: str | None = None


class OpenNebulaConfig(Model):
    version: str
    publicURL: str
    rpcEndpoint: str
    oneadminUser: str
    oneadminPassword: str
    serveradminSecretMode: str
    clusterName: str
    hosts: list[str]
    forceHypervisor: str
    template: TemplateConfig
    datastores: list[DatastoreConfig]
    networks: list[OpenNebulaNetworkConfig]


class DexStaticClient(Model):
    id: str
    secret: str
    redirectURIs: list[str]


class DexConfig(Model):
    publicURL: str
    username: str
    email: str
    password: str
    staticClients: list[DexStaticClient]


class OmniServiceAccountConfig(Model):
    name: str
    key: str


class OmniOIDCConfig(Model):
    issuerURL: str
    clientID: str
    clientSecret: str


class OmniConfig(Model):
    version: str
    publicURL: str
    machineAPIURL: str
    workloadProxyURL: str
    grpcTunnel: bool
    providerID: str
    adminEnvPath: str
    serviceAccount: OmniServiceAccountConfig
    oidc: OmniOIDCConfig


class DNSConfig(Model):
    provider: str
    zone: str
    cloudflareToken: str
    records: list[str]


class TLSConfig(Model):
    mode: str
    email: str
    storagePath: str
    certificateNames: list[str]


class ProviderRuntimeConfig(Model):
    providerID: str
    endpoint: str
    templateName: str
    hypervisor: str
    hostnameStrategy: str
    importOnMiss: bool
    requireChecksum: bool
    defaultDatastore: str
    allowedDatastores: list[str]
    allowedNetworks: list[str]
    allowExplicitResources: bool
    artifactURLTemplate: str


class ProviderConfig(Model):
    deployMode: str
    workingDir: str
    binaryMode: str
    image: str
    env: dict[str, str]
    config: ProviderRuntimeConfig


class HandoffClusterConfig(Model):
    enabled: bool = True
    name: str
    talosVersion: str
    kubernetesVersion: str
    controlPlanes: int
    workers: int
    rootDiskGiB: int
    preferredDatastore: str
    machineClassPrefix: str
    grpcTunnelKernelArg: bool = True


class ValidationConfig(Model):
    failFast: bool = True
    collectArtifactsOnFailure: bool = True


class SSHConfig(Model):
    privateKeyPath: str
    strictHostKeyChecking: str = "accept-new"
    connectTimeoutSeconds: int = 10


class ArtifactsConfig(Model):
    outputDir: str | None = None


class RootConfig(Model):
    schemaVersion: Literal["v1alpha1"]
    lab: LabConfig
    rackspace: RackspaceConfig
    nodes: NodesConfig
    networking: NetworkingConfig
    ceph: CephConfig
    opennebula: OpenNebulaConfig
    omni: OmniConfig
    dex: DexConfig
    dns: DNSConfig
    tls: TLSConfig
    provider: ProviderConfig
    handoffCluster: HandoffClusterConfig
    validation: ValidationConfig
    ssh: SSHConfig
    artifacts: ArtifactsConfig = Field(default_factory=ArtifactsConfig)

    @model_validator(mode="after")
    def validate_host_lists(self) -> RootConfig:
        configured = {node.hostname for node in self.nodes.all_nodes()}
        missing_hosts = [host for host in self.opennebula.hosts if host not in configured]
        if missing_hosts:
            host_list = ", ".join(missing_hosts)
            raise ValueError(f"opennebula.hosts not defined under nodes: {host_list}")
        missing_mons = [host for host in self.ceph.mons if host not in configured]
        if missing_mons:
            mon_list = ", ".join(missing_mons)
            raise ValueError(f"ceph.mons not defined under nodes: {mon_list}")
        return self

    @property
    def repo_root(self) -> Path:
        return Path(__file__).resolve().parents[4]

    @property
    def state_dir(self) -> Path:
        return self.repo_root / self.lab.stateDir

    @property
    def workspace_dir(self) -> Path:
        return self.repo_root / self.lab.workspaceDir

    @property
    def log_dir(self) -> Path:
        return self.repo_root / self.lab.logDir

    @property
    def rendered_dir(self) -> Path:
        return self.workspace_dir / "rendered"

    @property
    def state_path(self) -> Path:
        return self.state_dir / "state.json"

    def config_hash(self) -> str:
        payload = json.dumps(self.model_dump(mode="json"), sort_keys=True, separators=(",", ":"))
        return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def load_config(path: str | Path) -> RootConfig:
    config_path = Path(path).expanduser().resolve()
    data = yaml.safe_load(config_path.read_text())
    if not isinstance(data, dict):
        raise ValueError(f"expected YAML object in {config_path}")
    return RootConfig.model_validate(data)
