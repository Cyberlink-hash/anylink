package dbdata

import (
	"errors"
	"testing"

	"github.com/go-ldap/ldap"
	"github.com/stretchr/testify/assert"
)

func TestLDAPGroupMembershipEntryHelpers(t *testing.T) {
	ast := assert.New(t)
	userDN := "uid=dax,ou=people,dc=hotcoin,dc=biz"
	entry := &ldap.Entry{
		DN: "cn=vpn-users-market,ou=groups,dc=hotcoin,dc=biz",
		Attributes: []*ldap.EntryAttribute{
			{Name: "objectClass", Values: []string{"top", "groupOfUniqueNames"}},
			{Name: "uniqueMember", Values: []string{userDN}},
		},
	}

	ast.True(hasLDAPObjectClass(entry, "groupOfUniqueNames"))
	ast.True(hasLDAPAttributeValue(entry, "uniqueMember", userDN))
	ast.True(hasLDAPAttributeValue(entry, "uniqueMember", "UID=DAX,OU=PEOPLE,DC=HOTCOIN,DC=BIZ"))
	ast.False(hasLDAPObjectClass(entry, "groupOfNames"))
	ast.False(hasLDAPAttributeValue(entry, "member", userDN))
}

func TestLDAPMemberOfUnsupportedErrors(t *testing.T) {
	ast := assert.New(t)

	ast.True(isLDAPMemberOfUnsupported(ldap.NewError(ldap.LDAPResultUndefinedAttributeType, errors.New("memberOf"))))
	ast.True(isLDAPMemberOfUnsupported(ldap.NewError(ldap.LDAPResultUnwillingToPerform, errors.New("Unsupported user filter: memberOf"))))
	ast.True(isLDAPMemberOfUnsupported(errors.New("LDAP Result Code 53: Unsupported user filter memberOf")))
	ast.False(isLDAPMemberOfUnsupported(ldap.NewError(ldap.LDAPResultInvalidCredentials, errors.New("invalid credentials"))))
}
