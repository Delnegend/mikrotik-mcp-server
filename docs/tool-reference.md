# Tool reference

69 tools, grouped by area. Names below are the canonical `mcp.NewTool`
registration strings — use them verbatim. Every device-scoped tool also
accepts a `device` title; `jq_filter` applies to normalized `resource_print`
output.

### list_devices

| Group | Tools |
|---|---|
| Generic primitives | `resource_print`, `resource_add`, `resource_set`, `resource_remove`, `command_run`, `resource_listen`, `command_cancel` |
| Fleet & safe mode | `list_devices`, `safe_mode_status`, `enable_safe_mode`, `commit_safe_mode`, `rollback_safe_mode` |
| System & health | `healthcheck`, `system_resource_get`, `system_identity_get`, `system_clock_get` |

### resource_print

Generic slash-separated menus (`ip/address`, `interface/bridge/port`):
print with optional `proplist` + `jq_filter`; add/set/remove by `menu` plus
`attributes` / `item_id`. `command_run` takes a `command` path;
`resource_listen` returns a bounded event batch; `command_cancel` takes a `tag`.

### file_list

| Group | Tools |
|---|---|
| Files & backups | `file_list`, `file_download`, `system_backup_save`, `system_export`, `system_backup_collect` |
| Network utilities | `tool_ping`, `tool_traceroute`, `dns_resolve`, `interface_monitor` |

`file_download` needs `router_path` (optional `local_path`);
`system_backup_save` / `system_export` take a `name`; `tool_ping` and
`tool_traceroute` run bounded probes from the router.

### bridge_list

| Group | Tools |
|---|---|
| Bridges & VLANs | `bridge_list`, `bridge_add`, `bridge_remove`, `bridge_port_list`, `bridge_port_add`, `bridge_port_remove`, `bridge_vlan_list`, `bridge_vlan_add`, `bridge_vlan_remove`, `vlan_list`, `vlan_add`, `vlan_remove` |
| Firewall | `firewall_filter_list`, `firewall_filter_add`, `firewall_filter_set`, `firewall_filter_remove`, `firewall_nat_list`, `firewall_nat_add`, `firewall_nat_set`, `firewall_nat_remove`, `firewall_rule_move`, `firewall_address_list_list`, `firewall_address_list_add`, `firewall_address_list_remove` |

### firewall_filter_list

Filter/NAT lists accept `chain`, `action`, `disabled`; add/set take
`attributes`, remove takes `item_id`; `firewall_rule_move` takes `table`
(`filter` or `nat`) plus `item_id` and destination.

### ppp_secret_list

| Group | Tools |
|---|---|
| Addressing & routing | `interface_list`, `interface_get`, `ip_address_list`, `ip_address_get`, `ip_route_list`, `ip_route_get` |
| DHCP & DNS | `dhcp_lease_list`, `dhcp_server_list`, `dhcp_network_list`, `dns_get`, `dns_set` |
| PPP | `ppp_active_list`, `ppp_secret_list`, `ppp_secret_add`, `ppp_secret_remove` |
| WireGuard | `wireguard_interface_list`, `wireguard_interface_add`, `wireguard_peer_list`, `wireguard_peer_add`, `wireguard_peer_remove` |

### safe_mode_status

`enable_safe_mode` holds later mutations in memory; `commit_safe_mode`
persists them; `rollback_safe_mode` discards them (automatic on disconnect).
`safe_mode_status` reports whether the session is active.
