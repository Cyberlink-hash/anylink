package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/bjdgyc/anylink/dbdata"
)

func NetworkPolicyList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	pageS := r.FormValue("page")
	page, _ := strconv.Atoi(pageS)
	if page < 1 {
		page = 1
	}

	var pageSize = 0

	count := dbdata.CountAll(&dbdata.NetworkPolicy{})

	var datas []dbdata.NetworkPolicy
	err := dbdata.Find(&datas, pageSize, page)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	data := map[string]interface{}{
		"count":     count,
		"page_size": pageSize,
		"datas":     datas,
	}

	RespSucess(w, data)
}

func NetworkPolicyDetail(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)
	if id < 1 {
		RespError(w, RespParamErr, "Id错误")
		return
	}

	var data dbdata.NetworkPolicy
	err := dbdata.One("Id", id, &data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	RespSucess(w, data)
}

func NetworkPolicySet(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()
	v := &dbdata.NetworkPolicy{}
	err = json.Unmarshal(body, v)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	err = dbdata.SetNetworkPolicy(v)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	RespSucess(w, nil)
}

func NetworkPolicyDel(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)
	if id < 1 {
		RespError(w, RespParamErr, "Id错误")
		return
	}

	data := dbdata.NetworkPolicy{Id: id}
	err := dbdata.DeleteNetworkPolicy(data.Id)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	RespSucess(w, nil)
}

func NetworkPolicyUsers(w http.ResponseWriter, r *http.Request) {
	type option struct {
		Username string   `json:"username"`
		Nickname string   `json:"nickname"`
		Groups   []string `json:"groups"`
	}
	var users []dbdata.User
	query := "%" + strings.ToLower(r.URL.Query().Get("q")) + "%"
	err := dbdata.GetXdb().Cols("username", "nickname", "groups").Where("LOWER(username) LIKE ? OR LOWER(nickname) LIKE ?", query, query).Asc("username").Find(&users)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	values := []option{}
	for _, user := range users {
		values = append(values, option{user.Username, user.Nickname, user.Groups})
	}
	RespSucess(w, values)
}
