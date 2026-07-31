package server

import (
	"testing"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
	"github.com/Delnegend/mikrotik-mcp/internal/testutil"
)

// Generic resource handlers

func TestIntegrationResourceAdd(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done", "=ret=*42"))
	cl.SetConn(fc)

	result, err := handlerResourceAdd(cl)(ctx(), testutil.MkReq("bridge_add", "menu", "/interface/bridge", "name", "br-test"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
	assertSentExact(t, fc, []string{"/interface/bridge/add", "=name=br-test"})
}

func TestIntegrationResourceSet(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	result, err := handlerResourceSet(cl)(ctx(), testutil.MkReq("resource_set", "menu", "/ip/address", "item_id", "*4", "disabled", true))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
	assertSentExact(t, fc, []string{"/ip/address/set", "=.id=*4", "=disabled=true"})
}

func TestIntegrationResourceRemove(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!empty"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerResourceRemove(cl)(ctx(), testutil.MkReq("resource_remove", "menu", "/ip/address", "item_id", "*5"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
	assertSentExact(t, fc, []string{"/ip/address/remove", "=.id=*5"})
}

// Bridge

func TestIntegrationBridgeList(t *testing.T) {
	t.Run("filter by name", func(t *testing.T) {
		cl := client.NewRouterOSClient("router.test", "admin", "secret")
		fc := newFakeConn(enc("!re", "=name=br0"), enc("!done"))
		cl.SetConn(fc)
		_, err := filteredListHandler(cl, "/interface/bridge", map[string]string{"name": "name", "disabled": "disabled"})(ctx(), testutil.MkReq("bridge_list", "name", "br0"))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		assertSentExact(t, fc, []string{"/interface/bridge/print", "?name=br0"})
	})
}

func TestIntegrationBridgeAdd(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := addHandler(cl, "/interface/bridge")(ctx(), testutil.MkReq("bridge_add", "attributes", map[string]any{"name": "br1"}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/interface/bridge/add", "=name=br1"})
}

func TestIntegrationBridgeAddRequiresAttributes(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	_, err := addHandler(cl, "/interface/bridge")(ctx(), testutil.MkReq("bridge_add"))
	if err == nil {
		t.Fatal("expected error for missing attributes")
	}
}

func TestIntegrationBridgeRemove(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := removeHandler(cl, "/interface/bridge")(ctx(), testutil.MkReq("bridge_remove", "item_id", "*7"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/interface/bridge/remove", "=.id=*7"})
}

func TestIntegrationBridgePortList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := filteredListHandler(cl, "/interface/bridge/port", map[string]string{"bridge": "bridge", "interface": "interface", "disabled": "disabled"})(ctx(), testutil.MkReq("bridge_port_list", "bridge", "br0"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/interface/bridge/port/print", "?bridge=br0"})
}

func TestIntegrationBridgePortAdd(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := addHandler(cl, "/interface/bridge/port")(ctx(), testutil.MkReq("bridge_port_add", "attributes", map[string]any{"bridge": "br0", "interface": "ether1"}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/interface/bridge/port/add", "=bridge=br0", "=interface=ether1"})
}

func TestIntegrationBridgeVlanList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := filteredListHandler(cl, "/interface/bridge/vlan", map[string]string{"bridge": "bridge", "vlan_ids": "vlan-ids", "disabled": "disabled"})(ctx(), testutil.MkReq("bridge_vlan_list", "vlan_ids", "100"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/interface/bridge/vlan/print", "?vlan-ids=100"})
}

func TestIntegrationVlanList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := filteredListHandler(cl, "/interface/vlan", map[string]string{"name": "name", "interface": "interface", "disabled": "disabled"})(ctx(), testutil.MkReq("vlan_list", "interface", "bridge"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/interface/vlan/print", "?interface=bridge"})
}

// Firewall

func TestIntegrationFirewallFilterList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := filteredListHandler(cl, "/ip/firewall/filter", map[string]string{"chain": "chain", "action": "action", "disabled": "disabled"})(ctx(), testutil.MkReq("firewall_filter_list", "chain", "input", "action", "accept"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/ip/firewall/filter/print", "?action=accept", "?chain=input"})
}

func TestIntegrationFirewallFilterSet(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := setHandler(cl, "/ip/firewall/filter")(ctx(), testutil.MkReq("firewall_filter_set", "item_id", "*1", "attributes", map[string]any{"disabled": true}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/ip/firewall/filter/set", "=.id=*1", "=disabled=true"})
}

func TestIntegrationFirewallNatList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := filteredListHandler(cl, "/ip/firewall/nat", map[string]string{"chain": "chain", "action": "action", "disabled": "disabled"})(ctx(), testutil.MkReq("firewall_nat_list", "chain", "srcnat"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/ip/firewall/nat/print", "?chain=srcnat"})
}

func TestIntegrationFirewallRuleMove(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := firewallRuleMoveHandler(cl)(ctx(), testutil.MkReq("firewall_rule_move", "table", "filter", "item_id", "*2", "destination", "0"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/ip/firewall/filter/move", "=.id=*2", "=destination=0"})
}

func TestIntegrationFirewallRuleMoveInvalidTable(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	_, err := firewallRuleMoveHandler(cl)(ctx(), testutil.MkReq("firewall_rule_move", "table", "invalid", "item_id", "*1", "destination", "0"))
	if err == nil {
		t.Fatal("expected error for invalid table")
	}
}

func TestIntegrationFirewallAddressListList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := filteredListHandler(cl, "/ip/firewall/address-list", map[string]string{"list_name": "list", "address": "address", "disabled": "disabled"})(ctx(), testutil.MkReq("firewall_address_list_list", "list_name", "blocked"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/ip/firewall/address-list/print", "?list=blocked"})
}

// PPP

func TestIntegrationPPPActiveList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := filteredListHandler(cl, "/ppp/active", map[string]string{"service": "service", "name": "name"})(ctx(), testutil.MkReq("ppp_active_list", "service", "pppoe"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/ppp/active/print", "?service=pppoe"})
}

func TestIntegrationPPPSecretAdd(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := pppSecretAddHandler(cl)(ctx(), testutil.MkReq("ppp_secret_add", "attributes", map[string]any{"name": "user1", "password": "pass123"}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/ppp/secret/add", "=name=user1", "=password=pass123"})
}

func TestIntegrationPPPSecretAddRequiresFields(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	_, err := pppSecretAddHandler(cl)(ctx(), testutil.MkReq("ppp_secret_add", "attributes", map[string]any{"name": "user1"}))
	if err == nil {
		t.Fatal("expected error when password missing")
	}
}

// WireGuard

func TestIntegrationWireguardInterfaceList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := filteredListHandler(cl, "/interface/wireguard", map[string]string{"name": "name", "disabled": "disabled"})(ctx(), testutil.MkReq("wireguard_interface_list", "name", "wg0"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/interface/wireguard/print", "?name=wg0"})
}

func TestIntegrationWireguardPeerAdd(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := wgPeerAddHandler(cl)(ctx(), testutil.MkReq("wireguard_peer_add", "attributes", map[string]any{"interface": "wg0", "public-key": "abc123"}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/interface/wireguard/peers/add", "=interface=wg0", "=public-key=abc123"})
}

func TestIntegrationWireguardPeerAddRequiresFields(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	_, err := wgPeerAddHandler(cl)(ctx(), testutil.MkReq("wireguard_peer_add", "attributes", map[string]any{"interface": "wg0"}))
	if err == nil {
		t.Fatal("expected error when public-key missing")
	}
}
