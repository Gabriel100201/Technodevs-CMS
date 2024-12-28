package apis_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

func TestRealtimeConnect(t *testing.T) {
	scenarios := []tests.ApiScenario{
		{
			Method:         http.MethodGet,
			URL:            "/api/realtime",
			Timeout:        100 * time.Millisecond,
			ExpectedStatus: 200,
			ExpectedContent: []string{
				`id:`,
				`event:PB_CONNECT`,
				`data:{"clientId":`,
			},
			ExpectedEvents: map[string]int{
				"*":                        0,
				"OnRealtimeConnectRequest": 1,
				"OnRealtimeMessageSend":    1,
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if len(app.SubscriptionsBroker().Clients()) != 0 {
					t.Errorf("Expected the subscribers to be removed after connection close, found %d", len(app.SubscriptionsBroker().Clients()))
				}
			},
		},
		{
			Name:           "PB_CONNECT interrupt",
			Method:         http.MethodGet,
			URL:            "/api/realtime",
			Timeout:        100 * time.Millisecond,
			ExpectedStatus: 200,
			ExpectedEvents: map[string]int{
				"*":                        0,
				"OnRealtimeConnectRequest": 1,
				"OnRealtimeMessageSend":    1,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				app.OnRealtimeMessageSend().BindFunc(func(e *core.RealtimeMessageEvent) error {
					if e.Message.Name == "PB_CONNECT" {
						return errors.New("PB_CONNECT error")
					}
					return e.Next()
				})
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if len(app.SubscriptionsBroker().Clients()) != 0 {
					t.Errorf("Expected the subscribers to be removed after connection close, found %d", len(app.SubscriptionsBroker().Clients()))
				}
			},
		},
		{
			Name:           "Skipping/ignoring messages",
			Method:         http.MethodGet,
			URL:            "/api/realtime",
			Timeout:        100 * time.Millisecond,
			ExpectedStatus: 200,
			ExpectedEvents: map[string]int{
				"*":                        0,
				"OnRealtimeConnectRequest": 1,
				"OnRealtimeMessageSend":    1,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				app.OnRealtimeMessageSend().BindFunc(func(e *core.RealtimeMessageEvent) error {
					return nil
				})
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if len(app.SubscriptionsBroker().Clients()) != 0 {
					t.Errorf("Expected the subscribers to be removed after connection close, found %d", len(app.SubscriptionsBroker().Clients()))
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestRealtimeSubscribe(t *testing.T) {
	client := subscriptions.NewDefaultClient()

	resetClient := func() {
		client.Unsubscribe()
		client.Set(apis.RealtimeClientAuthKey, nil)
	}

	validSubscriptionsLimit := make([]string, 1000)
	for i := 0; i < len(validSubscriptionsLimit); i++ {
		validSubscriptionsLimit[i] = fmt.Sprintf(`"%d"`, i)
	}
	invalidSubscriptionsLimit := make([]string, 1001)
	for i := 0; i < len(invalidSubscriptionsLimit); i++ {
		invalidSubscriptionsLimit[i] = fmt.Sprintf(`"%d"`, i)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:            "missing client",
			Method:          http.MethodPost,
			URL:             "/api/realtime",
			Body:            strings.NewReader(`{"clientId":"missing","subscriptions":["test1", "test2"]}`),
			ExpectedStatus:  404,
			ExpectedContent: []string{`"data":{}`},
			ExpectedEvents:  map[string]int{"*": 0},
		},
		{
			Name:           "empty data",
			Method:         http.MethodPost,
			URL:            "/api/realtime",
			Body:           strings.NewReader(`{}`),
			ExpectedStatus: 400,
			ExpectedContent: []string{
				`"data":{`,
				`"clientId":{"code":"validation_required`,
			},
			NotExpectedContent: []string{
				`"subscriptions"`,
			},
			ExpectedEvents: map[string]int{"*": 0},
		},
		{
			Name:   "existing client with invalid subscriptions limit",
			Method: http.MethodPost,
			URL:    "/api/realtime",
			Body: strings.NewReader(`{
				"clientId": "` + client.Id() + `",
				"subscriptions": [` + strings.Join(invalidSubscriptionsLimit, ",") + `]
			}`),
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				app.SubscriptionsBroker().Register(client)
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				resetClient()
			},
			ExpectedStatus: 400,
			ExpectedContent: []string{
				`"data":{`,
				`"subscriptions":{"code":"validation_length_too_long"`,
			},
			ExpectedEvents: map[string]int{"*": 0},
		},
		{
			Name:   "existing client with valid subscriptions limit",
			Method: http.MethodPost,
			URL:    "/api/realtime",
			Body: strings.NewReader(`{
				"clientId": "` + client.Id() + `",
				"subscriptions": [` + strings.Join(validSubscriptionsLimit, ",") + `]
			}`),
			ExpectedStatus: 204,
			ExpectedEvents: map[string]int{
				"*":                          0,
				"OnRealtimeSubscribeRequest": 1,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				client.Subscribe("test0") // should be replaced
				app.SubscriptionsBroker().Register(client)
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if len(client.Subscriptions()) != len(validSubscriptionsLimit) {
					t.Errorf("Expected %d subscriptions, got %d", len(validSubscriptionsLimit), len(client.Subscriptions()))
				}
				if client.HasSubscription("test0") {
					t.Errorf("Expected old subscriptions to be replaced")
				}
				resetClient()
			},
		},
		{
			Name:   "existing client with invalid topic length",
			Method: http.MethodPost,
			URL:    "/api/realtime",
			Body: strings.NewReader(`{
				"clientId": "` + client.Id() + `",
				"subscriptions": ["abc", "` + strings.Repeat("a", 2501) + `"]
			}`),
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				app.SubscriptionsBroker().Register(client)
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				resetClient()
			},
			ExpectedStatus: 400,
			ExpectedContent: []string{
				`"data":{`,
				`"subscriptions":{"1":{"code":"validation_length_too_long"`,
			},
			ExpectedEvents: map[string]int{"*": 0},
		},
		{
			Name:   "existing client with valid topic length",
			Method: http.MethodPost,
			URL:    "/api/realtime",
			Body: strings.NewReader(`{
				"clientId": "` + client.Id() + `",
				"subscriptions": ["abc", "` + strings.Repeat("a", 2500) + `"]
			}`),
			ExpectedStatus: 204,
			ExpectedEvents: map[string]int{
				"*":                          0,
				"OnRealtimeSubscribeRequest": 1,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				client.Subscribe("test0")
				app.SubscriptionsBroker().Register(client)
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if len(client.Subscriptions()) != 2 {
					t.Errorf("Expected %d subscriptions, got %d", 2, len(client.Subscriptions()))
				}
				if client.HasSubscription("test0") {
					t.Errorf("Expected old subscriptions to be replaced")
				}
				resetClient()
			},
		},
		{
			Name:           "existing client - empty subscriptions",
			Method:         http.MethodPost,
			URL:            "/api/realtime",
			Body:           strings.NewReader(`{"clientId":"` + client.Id() + `","subscriptions":[]}`),
			ExpectedStatus: 204,
			ExpectedEvents: map[string]int{
				"*":                          0,
				"OnRealtimeSubscribeRequest": 1,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				client.Subscribe("test0")
				app.SubscriptionsBroker().Register(client)
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if len(client.Subscriptions()) != 0 {
					t.Errorf("Expected no subscriptions, got %d", len(client.Subscriptions()))
				}
				resetClient()
			},
		},
		{
			Name:           "existing client - 2 new subscriptions",
			Method:         http.MethodPost,
			URL:            "/api/realtime",
			Body:           strings.NewReader(`{"clientId":"` + client.Id() + `","subscriptions":["test1", "test2"]}`),
			ExpectedStatus: 204,
			ExpectedEvents: map[string]int{
				"*":                          0,
				"OnRealtimeSubscribeRequest": 1,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				client.Subscribe("test0")
				app.SubscriptionsBroker().Register(client)
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				expectedSubs := []string{"test1", "test2"}
				if len(expectedSubs) != len(client.Subscriptions()) {
					t.Errorf("Expected subscriptions %v, got %v", expectedSubs, client.Subscriptions())
				}

				for _, s := range expectedSubs {
					if !client.HasSubscription(s) {
						t.Errorf("Cannot find %q subscription in %v", s, client.Subscriptions())
					}
				}
				resetClient()
			},
		},
		{
			Name:   "existing client - guest -> authorized superuser",
			Method: http.MethodPost,
			URL:    "/api/realtime",
			Body:   strings.NewReader(`{"clientId":"` + client.Id() + `","subscriptions":["test1", "test2"]}`),
			Headers: map[string]string{
				"Authorization": "eyJhbGciOiJIUzI1NiJ9.eyJpZCI6InN5d2JoZWNuaDQ2cmhtMCIsInR5cGUiOiJhdXRoIiwiY29sbGVjdGlvbklkIjoicGJjXzMxNDI2MzU4MjMiLCJleHAiOjI1MjQ2MDQ0NjEsInJlZnJlc2hhYmxlIjp0cnVlfQ.UXgO3j-0BumcugrFjbd7j0M4MQvbrLggLlcu_YNGjoY",
			},
			ExpectedStatus: 204,
			ExpectedEvents: map[string]int{
				"*":                          0,
				"OnRealtimeSubscribeRequest": 1,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				app.SubscriptionsBroker().Register(client)
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				authRecord, _ := client.Get(apis.RealtimeClientAuthKey).(*core.Record)
				if authRecord == nil || !authRecord.IsSuperuser() {
					t.Errorf("Expected superuser auth record, got %v", authRecord)
				}
				resetClient()
			},
		},
		{
			Name:   "existing client - guest -> authorized regular auth record",
			Method: http.MethodPost,
			URL:    "/api/realtime",
			Body:   strings.NewReader(`{"clientId":"` + client.Id() + `","subscriptions":["test1", "test2"]}`),
			Headers: map[string]string{
				"Authorization": "eyJhbGciOiJIUzI1NiJ9.eyJpZCI6IjRxMXhsY2xtZmxva3UzMyIsInR5cGUiOiJhdXRoIiwiY29sbGVjdGlvbklkIjoiX3BiX3VzZXJzX2F1dGhfIiwiZXhwIjoyNTI0NjA0NDYxLCJyZWZyZXNoYWJsZSI6dHJ1ZX0.ZT3F0Z3iM-xbGgSG3LEKiEzHrPHr8t8IuHLZGGNuxLo",
			},
			ExpectedStatus: 204,
			ExpectedEvents: map[string]int{
				"*":                          0,
				"OnRealtimeSubscribeRequest": 1,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				app.SubscriptionsBroker().Register(client)
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				authRecord, _ := client.Get(apis.RealtimeClientAuthKey).(*core.Record)
				if authRecord == nil {
					t.Errorf("Expected regular user auth record, got %v", authRecord)
				}
				resetClient()
			},
		},
		{
			Name:   "existing client - same auth",
			Method: http.MethodPost,
			URL:    "/api/realtime",
			Body:   strings.NewReader(`{"clientId":"` + client.Id() + `","subscriptions":["test1", "test2"]}`),
			Headers: map[string]string{
				"Authorization": "eyJhbGciOiJIUzI1NiJ9.eyJpZCI6IjRxMXhsY2xtZmxva3UzMyIsInR5cGUiOiJhdXRoIiwiY29sbGVjdGlvbklkIjoiX3BiX3VzZXJzX2F1dGhfIiwiZXhwIjoyNTI0NjA0NDYxLCJyZWZyZXNoYWJsZSI6dHJ1ZX0.ZT3F0Z3iM-xbGgSG3LEKiEzHrPHr8t8IuHLZGGNuxLo",
			},
			ExpectedStatus: 204,
			ExpectedEvents: map[string]int{
				"*":                          0,
				"OnRealtimeSubscribeRequest": 1,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				// the same user as the auth token
				user, err := app.FindAuthRecordByEmail("users", "test@example.com")
				if err != nil {
					t.Fatal(err)
				}

				client.Set(apis.RealtimeClientAuthKey, user)

				app.SubscriptionsBroker().Register(client)
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				authRecord, _ := client.Get(apis.RealtimeClientAuthKey).(*core.Record)
				if authRecord == nil {
					t.Errorf("Expected auth record model, got nil")
				}
				resetClient()
			},
		},
		{
			Name:   "existing client - mismatched auth",
			Method: http.MethodPost,
			URL:    "/api/realtime",
			Body:   strings.NewReader(`{"clientId":"` + client.Id() + `","subscriptions":["test1", "test2"]}`),
			Headers: map[string]string{
				"Authorization": "eyJhbGciOiJIUzI1NiJ9.eyJpZCI6IjRxMXhsY2xtZmxva3UzMyIsInR5cGUiOiJhdXRoIiwiY29sbGVjdGlvbklkIjoiX3BiX3VzZXJzX2F1dGhfIiwiZXhwIjoyNTI0NjA0NDYxLCJyZWZyZXNoYWJsZSI6dHJ1ZX0.ZT3F0Z3iM-xbGgSG3LEKiEzHrPHr8t8IuHLZGGNuxLo",
			},
			ExpectedStatus:  403,
			ExpectedContent: []string{`"data":{}`},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				user, err := app.FindAuthRecordByEmail("users", "test2@example.com")
				if err != nil {
					t.Fatal(err)
				}

				client.Set(apis.RealtimeClientAuthKey, user)

				app.SubscriptionsBroker().Register(client)
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				authRecord, _ := client.Get(apis.RealtimeClientAuthKey).(*core.Record)
				if authRecord == nil {
					t.Errorf("Expected auth record model, got nil")
				}
				resetClient()
			},
		},
		{
			Name:            "existing client - unauthorized client",
			Method:          http.MethodPost,
			URL:             "/api/realtime",
			Body:            strings.NewReader(`{"clientId":"` + client.Id() + `","subscriptions":["test1", "test2"]}`),
			ExpectedStatus:  403,
			ExpectedContent: []string{`"data":{}`},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				user, err := app.FindAuthRecordByEmail("users", "test2@example.com")
				if err != nil {
					t.Fatal(err)
				}

				client.Set(apis.RealtimeClientAuthKey, user)

				app.SubscriptionsBroker().Register(client)
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				authRecord, _ := client.Get(apis.RealtimeClientAuthKey).(*core.Record)
				if authRecord == nil {
					t.Errorf("Expected auth record model, got nil")
				}
				resetClient()
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestRealtimeAuthRecordDeleteEvent(t *testing.T) {
	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	// init realtime handlers
	apis.NewRouter(testApp)

	authRecord1, err := testApp.FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	authRecord2, err := testApp.FindAuthRecordByEmail("users", "test2@example.com")
	if err != nil {
		t.Fatal(err)
	}

	client1 := subscriptions.NewDefaultClient()
	client1.Set(apis.RealtimeClientAuthKey, authRecord1)
	testApp.SubscriptionsBroker().Register(client1)

	client2 := subscriptions.NewDefaultClient()
	client2.Set(apis.RealtimeClientAuthKey, authRecord1)
	testApp.SubscriptionsBroker().Register(client2)

	client3 := subscriptions.NewDefaultClient()
	client3.Set(apis.RealtimeClientAuthKey, authRecord2)
	testApp.SubscriptionsBroker().Register(client3)

	// mock delete event
	e := new(core.ModelEvent)
	e.App = testApp
	e.Type = core.ModelEventTypeDelete
	e.Context = context.Background()
	e.Model = authRecord1

	testApp.OnModelAfterDeleteSuccess().Trigger(e)

	if total := len(testApp.SubscriptionsBroker().Clients()); total != 3 {
		t.Fatalf("Expected %d subscription clients, found %d", 3, total)
	}

	if auth := client1.Get(apis.RealtimeClientAuthKey); auth != nil {
		t.Fatalf("[client1] Expected the auth state to be unset, found %#v", auth)
	}

	if auth := client2.Get(apis.RealtimeClientAuthKey); auth != nil {
		t.Fatalf("[client2] Expected the auth state to be unset, found %#v", auth)
	}

	if auth := client3.Get(apis.RealtimeClientAuthKey); auth == nil || auth.(*core.Record).Id != authRecord2.Id {
		t.Fatalf("[client3] Expected the auth state to be left unchanged, found %#v", auth)
	}
}

func TestRealtimeAuthRecordUpdateEvent(t *testing.T) {
	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	// init realtime handlers
	apis.NewRouter(testApp)

	authRecord1, err := testApp.FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	client := subscriptions.NewDefaultClient()
	client.Set(apis.RealtimeClientAuthKey, authRecord1)
	testApp.SubscriptionsBroker().Register(client)

	// refetch the authRecord and change its email
	authRecord2, err := testApp.FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	authRecord2.SetEmail("new@example.com")

	// mock update event
	e := new(core.ModelEvent)
	e.App = testApp
	e.Type = core.ModelEventTypeUpdate
	e.Context = context.Background()
	e.Model = authRecord2

	testApp.OnModelAfterUpdateSuccess().Trigger(e)

	clientAuthRecord, _ := client.Get(apis.RealtimeClientAuthKey).(*core.Record)
	if clientAuthRecord.Email() != authRecord2.Email() {
		t.Fatalf("Expected authRecord with email %q, got %q", authRecord2.Email(), clientAuthRecord.Email())
	}
}

// Custom auth record model struct
// -------------------------------------------------------------------
var _ core.Model = (*CustomUser)(nil)

type CustomUser struct {
	core.BaseModel

	Email string `db:"email" json:"email"`
}

func (m *CustomUser) TableName() string {
	return "users"
}

func findCustomUserByEmail(app core.App, email string) (*CustomUser, error) {
	model := &CustomUser{}

	err := app.ModelQuery(model).
		AndWhere(dbx.HashExp{"email": email}).
		Limit(1).
		One(model)

	if err != nil {
		return nil, err
	}

	return model, nil
}

func TestRealtimeCustomAuthModelDeleteEvent(t *testing.T) {
	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	// init realtime handlers
	apis.NewRouter(testApp)

	authRecord1, err := testApp.FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	authRecord2, err := testApp.FindAuthRecordByEmail("users", "test2@example.com")
	if err != nil {
		t.Fatal(err)
	}

	client1 := subscriptions.NewDefaultClient()
	client1.Set(apis.RealtimeClientAuthKey, authRecord1)
	testApp.SubscriptionsBroker().Register(client1)

	client2 := subscriptions.NewDefaultClient()
	client2.Set(apis.RealtimeClientAuthKey, authRecord1)
	testApp.SubscriptionsBroker().Register(client2)

	client3 := subscriptions.NewDefaultClient()
	client3.Set(apis.RealtimeClientAuthKey, authRecord2)
	testApp.SubscriptionsBroker().Register(client3)

	// refetch the authRecord as CustomUser
	customUser, err := findCustomUserByEmail(testApp, authRecord1.Email())
	if err != nil {
		t.Fatal(err)
	}

	// delete the custom user (should unset the client auth record)
	if err := testApp.Delete(customUser); err != nil {
		t.Fatal(err)
	}

	if total := len(testApp.SubscriptionsBroker().Clients()); total != 3 {
		t.Fatalf("Expected %d subscription clients, found %d", 3, total)
	}

	if auth := client1.Get(apis.RealtimeClientAuthKey); auth != nil {
		t.Fatalf("[client1] Expected the auth state to be unset, found %#v", auth)
	}

	if auth := client2.Get(apis.RealtimeClientAuthKey); auth != nil {
		t.Fatalf("[client2] Expected the auth state to be unset, found %#v", auth)
	}

	if auth := client3.Get(apis.RealtimeClientAuthKey); auth == nil || auth.(*core.Record).Id != authRecord2.Id {
		t.Fatalf("[client3] Expected the auth state to be left unchanged, found %#v", auth)
	}
}

func TestRealtimeCustomAuthModelUpdateEvent(t *testing.T) {
	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	// init realtime handlers
	apis.NewRouter(testApp)

	authRecord, err := testApp.FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	client := subscriptions.NewDefaultClient()
	client.Set(apis.RealtimeClientAuthKey, authRecord)
	testApp.SubscriptionsBroker().Register(client)

	// refetch the authRecord as CustomUser
	customUser, err := findCustomUserByEmail(testApp, "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	// change its email
	customUser.Email = "new@example.com"
	if err := testApp.Save(customUser); err != nil {
		t.Fatal(err)
	}

	clientAuthRecord, _ := client.Get(apis.RealtimeClientAuthKey).(*core.Record)
	if clientAuthRecord.Email() != customUser.Email {
		t.Fatalf("Expected authRecord with email %q, got %q", customUser.Email, clientAuthRecord.Email())
	}
}