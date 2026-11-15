package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkarmy-cyber/darkphish/models"
)

func paginationFixture(t *testing.T) (*testContext, int64) {
	t.Helper()
	ctx := setupTest(t)
	// One campaign with 120 synthetic results so pagination boundaries are
	// exercised across three pages (50/50/20) plus the max-limit clamp.
	template := models.Template{Name: "pagination-tmpl", Subject: "p", Text: "t", UserId: ctx.admin.Id}
	if err := models.PostTemplate(&template); err != nil {
		t.Fatal(err)
	}
	page := models.Page{Name: "pagination-page", HTML: "<p>p</p>", UserId: ctx.admin.Id}
	if err := models.PostPage(&page); err != nil {
		t.Fatal(err)
	}
	smtp := models.SMTP{Name: "pagination-smtp", Host: "127.0.0.1:25", FromAddress: "p@example.invalid", UserId: ctx.admin.Id}
	if err := models.PostSMTP(&smtp); err != nil {
		t.Fatal(err)
	}
	group := models.Group{Name: "pagination-group", UserId: ctx.admin.Id}
	for i := 0; i < 120; i++ {
		group.Targets = append(group.Targets, models.Target{
			BaseRecipient: models.BaseRecipient{Email: fmt.Sprintf("target%03d@example.invalid", i), Position: "p"},
		})
	}
	if err := models.PostGroup(&group); err != nil {
		t.Fatal(err)
	}
	// GetGroupByName does not preload targets; attach the hydrated group so
	// PostCampaign materializes one result per participant.
	hydrated, err := models.GetGroup(group.Id, ctx.admin.Id)
	if err != nil {
		t.Fatal(err)
	}
	campaign := models.Campaign{
		Name: "pagination-campaign", UserId: ctx.admin.Id,
		Status:   models.CampaignInProgress,
		Template: models.Template{Name: template.Name},
		Page:     models.Page{Name: page.Name},
		SMTP:     models.SMTP{Name: smtp.Name},
		Groups:   []models.Group{hydrated},
	}
	if err := models.PostCampaign(&campaign, ctx.admin.Id); err != nil {
		t.Fatal(err)
	}
	return ctx, campaign.Id
}

type pageEnvelope struct {
	Id         int64           `json:"id"`
	Results    []models.Result `json:"results"`
	NextCursor string          `json:"next_cursor"`
	HasMore    bool            `json:"has_more"`
}

func fetchPage(t *testing.T, ctx *testContext, campaignID int64, query string) (*httptest.ResponseRecorder, pageEnvelope) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/campaigns/%d/results%s", campaignID, query), nil)
	req.Header.Set("Authorization", "Bearer "+ctx.apiKey)
	w := httptest.NewRecorder()
	ctx.apiServer.ServeHTTP(w, req)
	var envelope pageEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil && w.Code == http.StatusOK {
		t.Fatalf("decode page: %v (%s)", err, w.Body.String())
	}
	return w, envelope
}

func TestCampaignResultsPaginationWalksAllPagesWithCursor(t *testing.T) {
	ctx, campaignID := paginationFixture(t)

	w, first := fetchPage(t, ctx, campaignID, "?limit=50")
	if w.Code != http.StatusOK {
		t.Fatalf("first page: %d %s", w.Code, w.Body.String())
	}
	if len(first.Results) != 50 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page wrong: n=%d hasMore=%v cursor=%q", len(first.Results), first.HasMore, first.NextCursor)
	}
	seen := map[string]bool{}
	for _, result := range first.Results {
		seen[result.RId] = true
	}

	w, second := fetchPage(t, ctx, campaignID, "?limit=50&cursor="+first.NextCursor)
	if w.Code != http.StatusOK || len(second.Results) != 50 || !second.HasMore {
		t.Fatalf("second page wrong: %d n=%d", w.Code, len(second.Results))
	}
	for _, result := range second.Results {
		if seen[result.RId] {
			t.Fatal("cursor revisited a result from page one")
		}
		seen[result.RId] = true
	}

	w, third := fetchPage(t, ctx, campaignID, "?limit=50&cursor="+second.NextCursor)
	if w.Code != http.StatusOK || len(third.Results) != 20 || third.HasMore || third.NextCursor != "" {
		t.Fatalf("final page wrong: %d n=%d hasMore=%v", w.Code, len(third.Results), third.HasMore)
	}
	if len(seen)+len(third.Results) != 120 {
		t.Fatalf("pagination lost rows: walked=%d", len(seen)+len(third.Results))
	}
}

func TestCampaignResultsPaginationClampsAndDefaults(t *testing.T) {
	ctx, campaignID := paginationFixture(t)

	w, page := fetchPage(t, ctx, campaignID, "?limit=9999")
	// The clamp bounds the page at 200; the fixture holds 120 rows total.
	if w.Code != http.StatusOK || len(page.Results) != 120 || len(page.Results) > models.ResultPageMax {
		t.Fatalf("max clamp failed: %d n=%d", w.Code, len(page.Results))
	}
	w, page = fetchPage(t, ctx, campaignID, "?limit=")
	if w.Code != http.StatusOK || len(page.Results) != models.ResultPageDefault {
		t.Fatalf("default size failed: %d n=%d", w.Code, len(page.Results))
	}
}

func TestCampaignResultsPaginationRejectsBadCursor(t *testing.T) {
	ctx, campaignID := paginationFixture(t)
	for _, cursor := range []string{"not-base64!!!", base64.RawURLEncoding.EncodeToString([]byte("x:1")), base64.RawURLEncoding.EncodeToString([]byte("r:not-a-number"))} {
		w, _ := fetchPage(t, ctx, campaignID, "?limit=10&cursor="+cursor)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("bad cursor %q accepted: %d", cursor, w.Code)
		}
	}
}

func TestCampaignResultsLegacyWithoutLimitParam(t *testing.T) {
	ctx, campaignID := paginationFixture(t)
	w, _ := fetchPage(t, ctx, campaignID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("legacy response failed: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "\"timeline\"") {
		t.Fatal("legacy full-detail response lost its timeline")
	}
}
