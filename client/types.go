package client

// Tipos da Docker Engine API usados pela lib: listagem e inspect de containers
// e o payload de criação de container.

/*
	---------------------------
		DNS público
	---------------------------
*/

// Servidores DNS públicos usados por padrão nos containers.
const (
	DNSCloudflare = "1.1.1.1" // mais rápido e focado em privacidade
	DNSGoogle     = "8.8.8.8" // muito estável
)

// DefaultDNS é a lista aplicada aos containers criados quando nenhuma outra é
// informada: Cloudflare como primário, Google como fallback.
var DefaultDNS = []string{DNSCloudflare, DNSGoogle}

/*
	===========================
		CONTAINERS — list
	===========================
*/

// Port é uma porta publicada por um container (formato da listagem).
type Port struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort,omitempty"`
	Type        string `json:"Type"` // "tcp" | "udp" | "sctp"
}

// SummaryHostConfig é o HostConfig reduzido presente na listagem.
type SummaryHostConfig struct {
	NetworkMode string            `json:"NetworkMode,omitempty"`
	Annotations map[string]string `json:"Annotations,omitempty"`
}

// SummaryNetworkSettings é o bloco de rede reduzido de GET /containers/json.
type SummaryNetworkSettings struct {
	Networks map[string]EndpointSettings `json:"Networks"`
}

// Container é um item de GET /containers/json.
type Container struct {
	ID              string                  `json:"Id"`
	Names           []string                `json:"Names"`
	Image           string                  `json:"Image"`
	ImageID         string                  `json:"ImageID"`
	Command         string                  `json:"Command,omitempty"`
	Created         int64                   `json:"Created"` // unix seconds (do daemon)
	Ports           []Port                  `json:"Ports,omitempty"`
	SizeRw          int64                   `json:"SizeRw,omitempty"`
	SizeRootFs      int64                   `json:"SizeRootFs,omitempty"`
	Labels          map[string]string       `json:"Labels,omitempty"`
	State           string                  `json:"State"`  // "running", "exited", ...
	Status          string                  `json:"Status"` // "Up 2 hours", ...
	HostConfig      *SummaryHostConfig      `json:"HostConfig,omitempty"`
	NetworkSettings *SummaryNetworkSettings `json:"NetworkSettings,omitempty"`
	Mounts          []MountPoint            `json:"Mounts,omitempty"`
}

/*
	===========================
		CONTAINERS — inspect
	===========================
*/

// HealthcheckResult é uma execução do healthcheck registrada no inspect.
type HealthcheckResult struct {
	Start    string `json:"Start"`
	End      string `json:"End"`
	ExitCode int    `json:"ExitCode"`
	Output   string `json:"Output"`
}

// Health é o estado de saúde do container.
type Health struct {
	Status        string              `json:"Status"` // "none" | "starting" | "healthy" | "unhealthy"
	FailingStreak int                 `json:"FailingStreak"`
	Log           []HealthcheckResult `json:"Log"`
}

// ContainerState é o bloco State de GET /containers/{id}/json.
type ContainerState struct {
	Status     string  `json:"Status"`
	Running    bool    `json:"Running"`
	Paused     bool    `json:"Paused"`
	Restarting bool    `json:"Restarting"`
	OOMKilled  bool    `json:"OOMKilled"`
	Dead       bool    `json:"Dead"`
	Pid        int     `json:"Pid"`
	ExitCode   int     `json:"ExitCode"`
	Error      string  `json:"Error,omitempty"`
	StartedAt  string  `json:"StartedAt"`
	FinishedAt string  `json:"FinishedAt"`
	Health     *Health `json:"Health,omitempty"`
}

// HealthConfig é a configuração de healthcheck da imagem/container.
type HealthConfig struct {
	Test          []string `json:"Test,omitempty"`
	Interval      int64    `json:"Interval,omitempty"`
	Timeout       int64    `json:"Timeout,omitempty"`
	Retries       int      `json:"Retries,omitempty"`
	StartPeriod   int64    `json:"StartPeriod,omitempty"`
	StartInterval int64    `json:"StartInterval,omitempty"`
}

// ContainerConfig é o bloco Config de GET /containers/{id}/json
// (também usado em ImageInspect.Config).
type ContainerConfig struct {
	Hostname        string              `json:"Hostname,omitempty"`
	Domainname      string              `json:"Domainname,omitempty"`
	User            string              `json:"User,omitempty"`
	AttachStdin     bool                `json:"AttachStdin,omitempty"`
	AttachStdout    bool                `json:"AttachStdout,omitempty"`
	AttachStderr    bool                `json:"AttachStderr,omitempty"`
	ExposedPorts    map[string]struct{} `json:"ExposedPorts,omitempty"`
	Tty             bool                `json:"Tty,omitempty"`
	OpenStdin       bool                `json:"OpenStdin,omitempty"`
	StdinOnce       bool                `json:"StdinOnce,omitempty"`
	Env             []string            `json:"Env,omitempty"`
	Cmd             []string            `json:"Cmd,omitempty"`
	Healthcheck     *HealthConfig       `json:"Healthcheck,omitempty"`
	ArgsEscaped     bool                `json:"ArgsEscaped,omitempty"`
	Image           string              `json:"Image"`
	Volumes         map[string]struct{} `json:"Volumes,omitempty"`
	WorkingDir      string              `json:"WorkingDir,omitempty"`
	Entrypoint      []string            `json:"Entrypoint,omitempty"`
	NetworkDisabled bool                `json:"NetworkDisabled,omitempty"`
	MacAddress      string              `json:"MacAddress,omitempty"`
	OnBuild         []string            `json:"OnBuild,omitempty"`
	Labels          map[string]string   `json:"Labels,omitempty"`
	StopSignal      string              `json:"StopSignal,omitempty"`
	StopTimeout     *int                `json:"StopTimeout,omitempty"`
	Shell           []string            `json:"Shell,omitempty"`
}

// MountPoint é um ponto de montagem do container.
type MountPoint struct {
	Type        string `json:"Type"`
	Name        string `json:"Name,omitempty"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Driver      string `json:"Driver,omitempty"`
	Mode        string `json:"Mode"`
	RW          bool   `json:"RW"`
	Propagation string `json:"Propagation"`
}

// GraphDriverData descreve o storage driver de um container ou imagem.
type GraphDriverData struct {
	Name string            `json:"Name"`
	Data map[string]string `json:"Data"`
}

// LogConfig é a configuração de logging do container.
type LogConfig struct {
	Type   string            `json:"Type"`
	Config map[string]string `json:"Config,omitempty"`
}

// Ulimit é um limite de recurso (rlimit) do processo.
type Ulimit struct {
	Name string `json:"Name"`
	Soft int64  `json:"Soft"`
	Hard int64  `json:"Hard"`
}

// DeviceMapping mapeia um device do host para dentro do container.
type DeviceMapping struct {
	PathOnHost        string `json:"PathOnHost"`
	PathInContainer   string `json:"PathInContainer"`
	CgroupPermissions string `json:"CgroupPermissions"`
}

// RestartPolicy controla o reinício automático do container.
type RestartPolicy struct {
	Name              string `json:"Name"` // "no" | "always" | "unless-stopped" | "on-failure"
	MaximumRetryCount int    `json:"MaximumRetryCount,omitempty"`
}

// PortBinding é o lado host de um mapeamento de porta.
type PortBinding struct {
	HostIP   string `json:"HostIp,omitempty"`
	HostPort string `json:"HostPort"`
}

// HostConfig são as opções de runtime do container. Serve para o corpo de
// POST /containers/create e para o campo HostConfig do inspect.
type HostConfig struct {
	// Rede e volumes
	Binds        []string                 `json:"Binds,omitempty"`
	PortBindings map[string][]PortBinding `json:"PortBindings,omitempty"`
	NetworkMode  string                   `json:"NetworkMode,omitempty"`
	DNS          []string                 `json:"Dns,omitempty"`
	DNSOptions   []string                 `json:"DnsOptions,omitempty"`
	DNSSearch    []string                 `json:"DnsSearch,omitempty"`
	ExtraHosts   []string                 `json:"ExtraHosts,omitempty"`
	VolumesFrom  []string                 `json:"VolumesFrom,omitempty"`
	Links        []string                 `json:"Links,omitempty"`
	LogConfig    *LogConfig               `json:"LogConfig,omitempty"`

	// Ciclo de vida
	RestartPolicy *RestartPolicy `json:"RestartPolicy,omitempty"`
	AutoRemove    bool           `json:"AutoRemove,omitempty"`

	// Recursos
	Memory            int64    `json:"Memory,omitempty"`            // bytes
	MemoryReservation int64    `json:"MemoryReservation,omitempty"` // bytes
	MemorySwap        int64    `json:"MemorySwap,omitempty"`        // bytes
	NanoCpus          int64    `json:"NanoCpus,omitempty"`          // 1 vCPU = 1_000_000_000
	CPUShares         int64    `json:"CpuShares,omitempty"`
	CPUPeriod         int64    `json:"CpuPeriod,omitempty"`
	CPUQuota          int64    `json:"CpuQuota,omitempty"`
	CpusetCpus        string   `json:"CpusetCpus,omitempty"`
	CpusetMems        string   `json:"CpusetMems,omitempty"`
	PidsLimit         *int64   `json:"PidsLimit,omitempty"`
	Ulimits           []Ulimit `json:"Ulimits,omitempty"`

	// Segurança e permissões
	Privileged      bool              `json:"Privileged,omitempty"`
	CapAdd          []string          `json:"CapAdd,omitempty"`
	CapDrop         []string          `json:"CapDrop,omitempty"`
	SecurityOpt     []string          `json:"SecurityOpt,omitempty"`
	ReadonlyRootfs  bool              `json:"ReadonlyRootfs,omitempty"`
	Devices         []DeviceMapping   `json:"Devices,omitempty"`
	GroupAdd        []string          `json:"GroupAdd,omitempty"`
	IpcMode         string            `json:"IpcMode,omitempty"`
	PidMode         string            `json:"PidMode,omitempty"`
	UTSMode         string            `json:"UTSMode,omitempty"`
	UsernsMode      string            `json:"UsernsMode,omitempty"`
	ShmSize         int64             `json:"ShmSize,omitempty"`
	Sysctls         map[string]string `json:"Sysctls,omitempty"`
	Runtime         string            `json:"Runtime,omitempty"`
	Isolation       string            `json:"Isolation,omitempty"`
	PublishAllPorts bool              `json:"PublishAllPorts,omitempty"`

	// Só aparecem no inspect
	ContainerIDFile string   `json:"ContainerIDFile,omitempty"`
	CgroupParent    string   `json:"CgroupParent,omitempty"`
	MaskedPaths     []string `json:"MaskedPaths,omitempty"`
	ReadonlyPaths   []string `json:"ReadonlyPaths,omitempty"`
}

// EndpointIPAMConfig fixa endereços do container numa rede.
type EndpointIPAMConfig struct {
	IPv4Address  string   `json:"IPv4Address,omitempty"`
	IPv6Address  string   `json:"IPv6Address,omitempty"`
	LinkLocalIPs []string `json:"LinkLocalIPs,omitempty"`
}

// EndpointSettings é a configuração do container numa rede específica.
type EndpointSettings struct {
	IPAMConfig          *EndpointIPAMConfig `json:"IPAMConfig,omitempty"`
	Links               []string            `json:"Links,omitempty"`
	Aliases             []string            `json:"Aliases,omitempty"`
	NetworkID           string              `json:"NetworkID"`
	EndpointID          string              `json:"EndpointID"`
	Gateway             string              `json:"Gateway"`
	IPAddress           string              `json:"IPAddress"`
	IPPrefixLen         int                 `json:"IPPrefixLen"`
	IPv6Gateway         string              `json:"IPv6Gateway"`
	GlobalIPv6Address   string              `json:"GlobalIPv6Address"`
	GlobalIPv6PrefixLen int                 `json:"GlobalIPv6PrefixLen"`
	MacAddress          string              `json:"MacAddress"`
	DriverOpts          map[string]string   `json:"DriverOpts,omitempty"`
	DNSNames            []string            `json:"DNSNames,omitempty"`
}

// Address é um par IP/prefixo.
type Address struct {
	Addr      string `json:"Addr"`
	PrefixLen int    `json:"PrefixLen"`
}

// NetworkSettings é o bloco de rede completo de GET /containers/{id}/json.
type NetworkSettings struct {
	Bridge                 string                      `json:"Bridge"`
	SandboxID              string                      `json:"SandboxID"`
	HairpinMode            bool                        `json:"HairpinMode"`
	LinkLocalIPv6Address   string                      `json:"LinkLocalIPv6Address"`
	LinkLocalIPv6PrefixLen int                         `json:"LinkLocalIPv6PrefixLen"`
	Ports                  map[string][]PortBinding    `json:"Ports"`
	SandboxKey             string                      `json:"SandboxKey"`
	SecondaryIPAddresses   []Address                   `json:"SecondaryIPAddresses,omitempty"`
	SecondaryIPv6Addresses []Address                   `json:"SecondaryIPv6Addresses,omitempty"`
	EndpointID             string                      `json:"EndpointID"`
	Gateway                string                      `json:"Gateway"`
	GlobalIPv6Address      string                      `json:"GlobalIPv6Address"`
	GlobalIPv6PrefixLen    int                         `json:"GlobalIPv6PrefixLen"`
	IPAddress              string                      `json:"IPAddress"`
	IPPrefixLen            int                         `json:"IPPrefixLen"`
	IPv6Gateway            string                      `json:"IPv6Gateway"`
	MacAddress             string                      `json:"MacAddress"`
	Networks               map[string]EndpointSettings `json:"Networks"`
}

// Inspect é a resposta completa de GET /containers/{id}/json.
type Inspect struct {
	ID              string           `json:"Id"`
	Created         string           `json:"Created"`
	Path            string           `json:"Path"`
	Args            []string         `json:"Args"`
	State           *ContainerState  `json:"State"`
	Image           string           `json:"Image"`
	ResolvConfPath  string           `json:"ResolvConfPath"`
	HostnamePath    string           `json:"HostnamePath"`
	HostsPath       string           `json:"HostsPath"`
	LogPath         string           `json:"LogPath"`
	Name            string           `json:"Name"`
	RestartCount    int              `json:"RestartCount"`
	Driver          string           `json:"Driver"`
	Platform        string           `json:"Platform"`
	MountLabel      string           `json:"MountLabel"`
	ProcessLabel    string           `json:"ProcessLabel"`
	AppArmorProfile string           `json:"AppArmorProfile"`
	ExecIDs         []string         `json:"ExecIDs"`
	HostConfig      *HostConfig      `json:"HostConfig"`
	GraphDriver     *GraphDriverData `json:"GraphDriver"`
	SizeRw          *int64           `json:"SizeRw,omitempty"`
	SizeRootFs      *int64           `json:"SizeRootFs,omitempty"`
	Mounts          []MountPoint     `json:"Mounts"`
	Config          *ContainerConfig `json:"Config"`
	NetworkSettings *NetworkSettings `json:"NetworkSettings"`
}

/*
	===========================
		CONTAINERS — create
	===========================
*/

// CreateContainer é o corpo de POST /containers/create.
type CreateContainer struct {
	Image        string              `json:"Image"`
	Env          []string            `json:"Env,omitempty"`
	Cmd          []string            `json:"Cmd,omitempty"`
	Entrypoint   []string            `json:"Entrypoint,omitempty"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts,omitempty"`
	Labels       map[string]string   `json:"Labels,omitempty"`
	HostConfig   *HostConfig         `json:"HostConfig,omitempty"`
}

// ensureDNS aplica DefaultDNS se nenhum DNS foi definido. Trabalha numa cópia
// do HostConfig, para não alterar o que quem chamou passou.
func (c *CreateContainer) ensureDNS() {
	var hc HostConfig
	if c.HostConfig != nil {
		hc = *c.HostConfig
	}
	if len(hc.DNS) == 0 {
		hc.DNS = append([]string(nil), DefaultDNS...)
	}
	c.HostConfig = &hc
}

// CreateResponse é a resposta de POST /containers/create.
type CreateResponse struct {
	ID       string   `json:"Id"`       // Id completo (64 caracteres hex)
	Warnings []string `json:"Warnings"` // avisos do Docker (ex.: limite ignorado)
}
