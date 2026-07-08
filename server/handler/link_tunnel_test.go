package handler

import (
	"testing"

	"github.com/bjdgyc/anylink/dbdata"
	"github.com/stretchr/testify/assert"
)

func TestApplyUserPolicyKeepsGroupSplitDnsWhenPolicyHasNoSplitDns(t *testing.T) {
	ast := assert.New(t)

	g := &dbdata.Group{
		NoGlobalDns: true,
		ClientDns:   []dbdata.ValData{{Val: "10.0.0.53"}},
		SplitDns:    []dbdata.ValData{{Val: "abc.com"}},
	}
	p := &dbdata.Policy{
		ClientDns: []dbdata.ValData{{Val: "10.0.0.54"}},
	}

	applyUserPolicy(g, p)

	ast.True(g.NoGlobalDns)
	ast.Equal([]dbdata.ValData{{Val: "abc.com"}}, g.SplitDns)
	ast.Equal([]dbdata.ValData{{Val: "10.0.0.54"}}, g.ClientDns)
}

func TestApplyUserPolicyOverridesGroupSplitDnsWhenPolicyHasSplitDns(t *testing.T) {
	ast := assert.New(t)

	g := &dbdata.Group{
		NoGlobalDns: true,
		SplitDns:    []dbdata.ValData{{Val: "abc.com"}},
	}
	p := &dbdata.Policy{
		NoGlobalDns: true,
		SplitDns:    []dbdata.ValData{{Val: "user.abc.com"}},
	}

	applyUserPolicy(g, p)

	ast.True(g.NoGlobalDns)
	ast.Equal([]dbdata.ValData{{Val: "user.abc.com"}}, g.SplitDns)
}
