package weworkcontactapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRemarkExternalContactGetsTokenAndPostsRemark(t *testing.T) {
	var tokenCalls int
	var remarkBodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/gettoken":
			tokenCalls++
			if request.URL.Query().Get("corpid") != "corp-1" || request.URL.Query().Get("corpsecret") != "secret-1" {
				t.Fatalf("token query = %s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-1","expires_in":7200}`))
		case "/externalcontact/remark":
			if request.URL.Query().Get("access_token") != "token-1" {
				t.Fatalf("remark token = %s", request.URL.RawQuery)
			}
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			remarkBodies = append(remarkBodies, body)
			_, _ = writer.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, Now: func() time.Time {
		return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	}}

	for index := 0; index < 2; index++ {
		err := client.RemarkExternalContact(context.Background(), RemarkRequest{
			EnterpriseID:   "ent-1",
			CorpID:         " corp-1 ",
			CorpSecret:     " secret-1 ",
			UserID:         " dy1 ",
			ExternalUserID: " ext-1 ",
			Remark:         " Alice#QWE ",
		})
		if err != nil {
			t.Fatalf("RemarkExternalContact returned error: %v", err)
		}
	}
	if tokenCalls != 1 {
		t.Fatalf("tokenCalls = %d, want cached token reuse", tokenCalls)
	}
	if len(remarkBodies) != 2 {
		t.Fatalf("remarkBodies = %#v", remarkBodies)
	}
	body := remarkBodies[0]
	if body["userid"] != "dy1" || body["external_userid"] != "ext-1" || body["remark"] != "Alice#QWE" {
		t.Fatalf("remark body = %#v", body)
	}
	if _, ok := body["description"]; ok {
		t.Fatalf("description should be omitted by default: %#v", body)
	}
}

func TestGetExternalContactGetsTokenAndFetchesPayload(t *testing.T) {
	var tokenCalls int
	var getCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/gettoken":
			tokenCalls++
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-1","expires_in":7200}`))
		case "/externalcontact/get":
			getCalls++
			if request.URL.Query().Get("access_token") != "token-1" || request.URL.Query().Get("external_userid") != "wm-1" {
				t.Fatalf("get query = %s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"external_contact":{"external_userid":"wm-1","name":"Alice"},"follow_user":[{"userid":"dy1","remark":"Alice"}]}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, Now: func() time.Time {
		return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	}}

	for index := 0; index < 2; index++ {
		payload, err := client.GetExternalContact(context.Background(), GetExternalContactRequest{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CorpSecret:     "secret-1",
			ExternalUserID: " wm-1 ",
		})
		if err != nil {
			t.Fatalf("GetExternalContact returned error: %v", err)
		}
		contact := payload["external_contact"].(map[string]any)
		if contact["name"] != "Alice" {
			t.Fatalf("payload = %#v", payload)
		}
	}
	if tokenCalls != 1 || getCalls != 2 {
		t.Fatalf("tokenCalls=%d getCalls=%d", tokenCalls, getCalls)
	}
}

func TestGetExternalContactReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/gettoken":
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-1","expires_in":7200}`))
		case "/externalcontact/get":
			_, _ = writer.Write([]byte(`{"errcode":40003,"errmsg":"invalid external_userid"}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL}

	_, err := client.GetExternalContact(context.Background(), GetExternalContactRequest{
		EnterpriseID:   "ent-1",
		CorpID:         "corp-1",
		CorpSecret:     "secret-1",
		ExternalUserID: "wm-1",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid external_userid") {
		t.Fatalf("error = %v", err)
	}
}

func TestExternalCorpTagsGetsCreatesAndMarksTags(t *testing.T) {
	var tokenCalls int
	var paths []string
	var addBody map[string]any
	var markBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/gettoken":
			tokenCalls++
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-1","expires_in":7200}`))
		case "/externalcontact/get_corp_tag_list":
			paths = append(paths, request.URL.Path)
			if request.URL.Query().Get("access_token") != "token-1" {
				t.Fatalf("tag list query = %s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"tag_group":[{"group_id":"group-1","group_name":"消息端工作台","tag":[{"id":"tag-1","name":"VIP"}]}]}`))
		case "/externalcontact/add_corp_tag":
			paths = append(paths, request.URL.Path)
			if request.URL.Query().Get("access_token") != "token-1" {
				t.Fatalf("add tag query = %s", request.URL.RawQuery)
			}
			if err := json.NewDecoder(request.Body).Decode(&addBody); err != nil {
				t.Fatalf("decode add body: %v", err)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		case "/externalcontact/mark_tag":
			paths = append(paths, request.URL.Path)
			if request.URL.Query().Get("access_token") != "token-1" {
				t.Fatalf("mark tag query = %s", request.URL.RawQuery)
			}
			if err := json.NewDecoder(request.Body).Decode(&markBody); err != nil {
				t.Fatalf("decode mark body: %v", err)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, Now: func() time.Time {
		return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	}}

	payload, err := client.GetExternalCorpTagList(context.Background(), CorpTagListRequest{EnterpriseID: "ent-1", CorpID: "corp-1", CorpSecret: "secret-1"})
	if err != nil {
		t.Fatalf("GetExternalCorpTagList returned error: %v", err)
	}
	if len(mapSlice(payload["tag_group"])) != 1 {
		t.Fatalf("payload = %#v", payload)
	}
	err = client.AddExternalCorpTags(context.Background(), AddCorpTagsRequest{
		EnterpriseID: "ent-1",
		CorpID:       "corp-1",
		CorpSecret:   "secret-1",
		TagNames:     []string{" New ", "new", "Important"},
		GroupID:      " group-1 ",
		GroupName:    "ignored",
	})
	if err != nil {
		t.Fatalf("AddExternalCorpTags returned error: %v", err)
	}
	err = client.MarkExternalContactTags(context.Background(), MarkExternalContactTagsRequest{
		EnterpriseID:   "ent-1",
		CorpID:         "corp-1",
		CorpSecret:     "secret-1",
		UserID:         " dy1 ",
		ExternalUserID: " wm-1 ",
		AddTagIDs:      []string{" tag-2 ", "tag-2"},
		RemoveTagIDs:   []string{" tag-1 "},
	})
	if err != nil {
		t.Fatalf("MarkExternalContactTags returned error: %v", err)
	}
	if tokenCalls != 1 || len(paths) != 3 {
		t.Fatalf("tokenCalls=%d paths=%#v", tokenCalls, paths)
	}
	tags := addBody["tag"].([]any)
	if addBody["group_id"] != "group-1" || len(tags) != 2 || tags[0].(map[string]any)["name"] != "New" || tags[1].(map[string]any)["name"] != "Important" {
		t.Fatalf("add body = %#v", addBody)
	}
	addTags := markBody["add_tag"].([]any)
	removeTags := markBody["remove_tag"].([]any)
	if markBody["userid"] != "dy1" || markBody["external_userid"] != "wm-1" || len(addTags) != 1 || addTags[0] != "tag-2" || len(removeTags) != 1 || removeTags[0] != "tag-1" {
		t.Fatalf("mark body = %#v", markBody)
	}
}

func TestListExternalContactIDsGetsTokenAndReturnsIDs(t *testing.T) {
	var tokenCalls int
	var listCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/gettoken":
			tokenCalls++
			if request.URL.Query().Get("corpid") != "corp-1" || request.URL.Query().Get("corpsecret") != "secret-1" {
				t.Fatalf("token query = %s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-1","expires_in":7200}`))
		case "/externalcontact/list":
			listCalls++
			if request.URL.Query().Get("access_token") != "token-1" || request.URL.Query().Get("userid") != "dy1" {
				t.Fatalf("list query = %s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"external_userid":[" wm-1 ","",123,"wm-2"]}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, Now: func() time.Time {
		return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	}}

	for index := 0; index < 2; index++ {
		ids, err := client.ListExternalContactIDs(context.Background(), ListExternalContactIDsRequest{
			EnterpriseID: "ent-1",
			CorpID:       " corp-1 ",
			CorpSecret:   " secret-1 ",
			UserID:       " dy1 ",
		})
		if err != nil {
			t.Fatalf("ListExternalContactIDs returned error: %v", err)
		}
		if len(ids) != 3 || ids[0] != "wm-1" || ids[1] != "123" || ids[2] != "wm-2" {
			t.Fatalf("ids = %#v", ids)
		}
	}
	if tokenCalls != 1 || listCalls != 2 {
		t.Fatalf("tokenCalls=%d listCalls=%d", tokenCalls, listCalls)
	}
}

func TestListExternalContactIDsSkipsKnownNoExternalContactErrors(t *testing.T) {
	for _, body := range []string{
		`{"errcode":84061,"errmsg":"not external contact"}`,
		`{"errcode":40003,"errmsg":"Not External Contact: no permission"}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/gettoken":
					_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-1","expires_in":7200}`))
				case "/externalcontact/list":
					_, _ = writer.Write([]byte(body))
				default:
					t.Fatalf("unexpected path %s", request.URL.Path)
				}
			}))
			defer server.Close()
			client := &Client{BaseURL: server.URL}

			ids, err := client.ListExternalContactIDs(context.Background(), ListExternalContactIDsRequest{
				EnterpriseID: "ent-1",
				CorpID:       "corp-1",
				CorpSecret:   "secret-1",
				UserID:       "dy1",
			})
			if err != nil {
				t.Fatalf("ListExternalContactIDs returned error: %v", err)
			}
			if len(ids) != 0 {
				t.Fatalf("ids = %#v", ids)
			}
		})
	}
}

func TestListExternalContactIDsReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/gettoken":
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-1","expires_in":7200}`))
		case "/externalcontact/list":
			_, _ = writer.Write([]byte(`{"errcode":40003,"errmsg":"invalid userid"}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL}

	_, err := client.ListExternalContactIDs(context.Background(), ListExternalContactIDsRequest{
		EnterpriseID: "ent-1",
		CorpID:       "corp-1",
		CorpSecret:   "secret-1",
		UserID:       "dy1",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid userid") {
		t.Fatalf("error = %v", err)
	}
}

func TestListInternalUsersGetsTokenAndReturnsUserlist(t *testing.T) {
	var tokenCalls int
	var listCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/gettoken":
			tokenCalls++
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-1","expires_in":7200}`))
		case "/user/simplelist":
			listCalls++
			query := request.URL.Query()
			if query.Get("access_token") != "token-1" || query.Get("department_id") != "1" || query.Get("fetch_child") != "1" {
				t.Fatalf("simplelist query = %s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"userlist":[{"userid":"dy1","name":"张三"},"skip",{"userid":"dy2","name":"李四"}]}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, Now: func() time.Time {
		return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	}}

	for index := 0; index < 2; index++ {
		users, err := client.ListInternalUsers(context.Background(), ListInternalUsersRequest{
			EnterpriseID: "ent-1",
			CorpID:       " corp-1 ",
			CorpSecret:   " secret-1 ",
		})
		if err != nil {
			t.Fatalf("ListInternalUsers returned error: %v", err)
		}
		if len(users) != 2 || users[0]["userid"] != "dy1" || users[1]["userid"] != "dy2" {
			t.Fatalf("users = %#v", users)
		}
	}
	if tokenCalls != 1 || listCalls != 2 {
		t.Fatalf("tokenCalls=%d listCalls=%d", tokenCalls, listCalls)
	}
}

func TestListInternalUsersReturnsEmptyOnAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/gettoken":
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-1","expires_in":7200}`))
		case "/user/simplelist":
			_, _ = writer.Write([]byte(`{"errcode":60011,"errmsg":"no permission"}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL}

	users, err := client.ListInternalUsers(context.Background(), ListInternalUsersRequest{
		EnterpriseID: "ent-1",
		CorpID:       "corp-1",
		CorpSecret:   "secret-1",
	})
	if err != nil {
		t.Fatalf("ListInternalUsers returned error: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("users = %#v", users)
	}
}

func TestGetInternalUserGetsTokenAndFetchesPayload(t *testing.T) {
	var getCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/gettoken":
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-1","expires_in":7200}`))
		case "/user/get":
			getCalls++
			if request.URL.Query().Get("access_token") != "token-1" || request.URL.Query().Get("userid") != "dy1" {
				t.Fatalf("user/get query = %s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"userid":"dy1","name":"张三","department":[1,2]}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL}

	payload, err := client.GetInternalUser(context.Background(), GetInternalUserRequest{
		EnterpriseID: "ent-1",
		CorpID:       "corp-1",
		CorpSecret:   "secret-1",
		UserID:       " dy1 ",
	})
	if err != nil {
		t.Fatalf("GetInternalUser returned error: %v", err)
	}
	if payload["userid"] != "dy1" || payload["name"] != "张三" || getCalls != 1 {
		t.Fatalf("payload=%#v getCalls=%d", payload, getCalls)
	}
}

func TestGetInternalUserReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/gettoken":
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-1","expires_in":7200}`))
		case "/user/get":
			_, _ = writer.Write([]byte(`{"errcode":60111,"errmsg":"userid not found"}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL}

	_, err := client.GetInternalUser(context.Background(), GetInternalUserRequest{
		EnterpriseID: "ent-1",
		CorpID:       "corp-1",
		CorpSecret:   "secret-1",
		UserID:       "dy1",
	})
	if err == nil || !strings.Contains(err.Error(), "userid not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestRemarkExternalContactReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/gettoken":
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"token-1","expires_in":7200}`))
		case "/externalcontact/remark":
			_, _ = writer.Write([]byte(`{"errcode":40003,"errmsg":"invalid external_userid"}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL}

	err := client.RemarkExternalContact(context.Background(), RemarkRequest{
		EnterpriseID:   "ent-1",
		CorpID:         "corp-1",
		CorpSecret:     "secret-1",
		UserID:         "dy1",
		ExternalUserID: "ext-1",
		Remark:         "Alice#QWE",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid external_userid") {
		t.Fatalf("error = %v", err)
	}
}
