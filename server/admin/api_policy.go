package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/bjdgyc/anylink/dbdata"
)

func PolicyList(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	pageS := r.FormValue("page")
	page, _ := strconv.Atoi(pageS)
	if page < 1 {
		page = 1
	}

	var pageSize = dbdata.PageSize

	session := dbdata.GetXdb().NewSession()
	defer session.Close()
	if search := strings.TrimSpace(r.FormValue("search")); search != "" {
		session.And("LOWER(username) LIKE ?", "%"+strings.ToLower(search)+"%")
	}
	if scope := r.FormValue("group_id"); scope != "" {
		id, err := strconv.Atoi(scope)
		if err != nil || id < 0 {
			RespError(w, RespParamErr, "用户组筛选错误")
			return
		}
		session.And("group_id = ?", id)
	}
	var datas []dbdata.Policy
	count, err := session.Desc("id").Limit(pageSize, (page-1)*pageSize).FindAndCount(&datas)
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

func PolicyDetail(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)
	if id < 1 {
		RespError(w, RespParamErr, "Id错误")
		return
	}

	var data dbdata.Policy
	err := dbdata.One("Id", id, &data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	dbdata.CompatPolicyDns(&data)

	RespSucess(w, data)
}

func PolicySet(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()
	v := &dbdata.Policy{}
	err = json.Unmarshal(body, v)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	err = dbdata.SetPolicy(v)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	RespSucess(w, nil)
}

func PolicyDel(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idS := r.FormValue("id")
	id, _ := strconv.Atoi(idS)
	if id < 1 {
		RespError(w, RespParamErr, "Id错误")
		return
	}

	data := dbdata.Policy{Id: id}
	err := dbdata.Del(&data)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	RespSucess(w, nil)
}

func PolicyPreview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string         `json:"username"`
		GroupId  int            `json:"group_id"`
		Policy   *dbdata.Policy `json:"policy"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		RespError(w, RespParamErr, "预览参数错误")
		return
	}
	result, err := dbdata.PreviewPolicy(input.Username, input.GroupId, input.Policy)
	if err != nil {
		RespError(w, RespParamErr, err)
		return
	}
	RespSucess(w, result)
}
