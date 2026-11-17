export type SystemInfo = {
  hostname: string;
  os: string;
  distro: string;
  distro_version: string;
  kernel: string;
  cpu_model: string;
  cpu_cores: number;
  uptime_seconds: number;
  private_ips: string[];
  public_ip?: string;
  app_version: string;
};

export type Disk = {
  mount: string;
  filesystem: string;
  total: number;
  used: number;
  available: number;
  percent: number;
};

export type NetIface = {
  name: string;
  rx: number;
  tx: number;
  rx_bps: number;
  tx_bps: number;
};

export type Metrics = {
  current: {
    time: string;
    cpu_percent: number;
    cpu_cores: number;
    load1: number;
    load5: number;
    load15: number;
    mem_total: number;
    mem_used: number;
    mem_available: number;
    mem_percent: number;
    disks: Disk[];
    net: NetIface[];
    net_rx_bps: number;
    net_tx_bps: number;
  };
  history: { t: number; cpu: number; mem: number; rx: number; tx: number }[];
  refresh_interval: number;
  updated_at: string;
};

export type MySQLDatabase = {
  name: string;
  system: boolean;
};

export type MySQLUser = {
  name: string;
  host: string;
  system: boolean;
};

export type MySQLGrant = {
  user: string;
  host: string;
  database: string;
  privileges: string[];
  system: boolean;
};

export type PHPMyAdminStatus = {
  installed: boolean;
  version: string;
  enabled: boolean;
  mode: string;
};

export type MySQLAdminView = {
  databases: MySQLDatabase[];
  users: MySQLUser[];
  grants: MySQLGrant[];
  phpmyadmin: PHPMyAdminStatus;
  error?: string;
};

export type Engine = {
  id: string;
  label: string;
  installed: boolean;
  package: string;
  version: string;
  unit: string;
  active: string;
  enabled: string;
  unit_exists: boolean;
  config: Record<string, string>;
  package_manager: string;
};

export type NodeStatus = {
  nvm_installed: boolean;
  nvm_path: string;
  node_version: string;
  npm_version: string;
  default: string;
  versions: string[];
  global_packages: string[];
};

export type PM2Process = {
  id: number;
  name: string;
  status: string;
  cpu: number;
  memory: number;
  restarts: number;
  uptime: number;
  pid: number;
};

export type PM2Status = {
  installed: boolean;
  version: string;
  path: string;
  processes: PM2Process[];
};

export type FTPUser = {
  name: string;
  home: string;
  locked: boolean | null;
  write: boolean;
};

export type FTPSession = {
  local: string;
  peer: string;
};

export type FTPStatus = {
  installed: boolean;
  version: string;
  active: string;
  enabled: string;
  unit_exists: boolean;
  port: number;
  pasv_min: number;
  pasv_max: number;
  pasv_address: string;
  write: boolean;
  umask: string;
  idle_timeout: number;
  data_timeout: number;
  max_clients: number;
  max_per_ip: number;
  max_rate: number;
  banner: string;
  transfer_log: boolean;
  tls: boolean;
  tls_cert: string;
  tls_key: string;
  users: FTPUser[];
  sessions: FTPSession[];
};

export type Certificate = {
  name: string;
  cert: string;
  key: string;
  subject: string;
  issuer: string;
  not_after: string;
  days: number;
  managed: boolean;
};

export type NginxStatus = {
  installed: boolean;
  version: string;
  unit: string;
  active: string;
  enabled: string;
  exists: boolean;
  layout: string;
  manager: string;
};

export type Site = {
  name: string;
  enabled: boolean;
  path: string;
  config: string;
  domains: string;
};

export type Settings = {
  version: string;
  host: string;
  port: number;
  tls_cert: string;
  tls_key: string;
  refresh_interval: number;
  session_ttl_minutes: number;
  dry_run: boolean;
  database: DatabaseStatus;
};

export type DatabaseStatus = {
  driver: string;
  path: string;
  open: boolean;
  version: number;
  latest: number;
  ready: boolean;
};

export type AuditEntry = {
  id: number;
  created_at: number;
  actor: string;
  action: string;
  target: string;
  detail: string;
  ip: string;
};

export type ActionResult = {
  ok: boolean;
  dry_run: boolean;
  message: string;
  output: string;
};
