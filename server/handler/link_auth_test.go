package handler

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientAuthErrorHidesLDAPNetworkDetails(t *testing.T) {
	ast := assert.New(t)

	err := errors.New("test LDAP 管理员 DN或密码填写有误 unable to read LDAP response packet: read tcp 172.18.0.6:48676->172.18.0.7:3890: read: connection reset by peer")
	msg := clientAuthError(err)

	ast.Equal("test LDAP 管理员 DN或密码填写有误", msg)
	ast.False(strings.Contains(msg, "172.18.0.6"))
	ast.False(strings.Contains(msg, "172.18.0.7"))
	ast.False(strings.Contains(msg, "read tcp"))
}

func TestClientAuthErrorKeepsBusinessError(t *testing.T) {
	ast := assert.New(t)

	msg := clientAuthError(errors.New("test LDAP 登入失败，请检查登入的账号或密码 LDAP Result Code 49"))

	ast.Equal("test LDAP 登入失败，请检查登入的账号或密码 LDAP Result Code 49", msg)
}

func TestClientAuthErrorHidesDialAddrBeforeNetworkDetails(t *testing.T) {
	ast := assert.New(t)

	msg := clientAuthError(errors.New("LDAP连接失败 172.18.0.7:3890 dial tcp 172.18.0.7:3890: connect: connection refused"))

	ast.Equal("LDAP连接失败 [已隐藏地址]", msg)
	ast.False(strings.Contains(msg, "172.18.0.7"))
	ast.False(strings.Contains(msg, "3890"))
	ast.False(strings.Contains(msg, "dial tcp"))
}
