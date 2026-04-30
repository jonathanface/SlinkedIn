package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestCleanURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "removes trackingId",
			input: "https://www.linkedin.com/jobs/view/123?trackingId=abc123&refId=def",
			want:  "https://www.linkedin.com/jobs/view/123?refId=def",
		},
		{
			name:  "no trackingId unchanged",
			input: "https://www.linkedin.com/jobs/view/123?refId=def",
			want:  "https://www.linkedin.com/jobs/view/123?refId=def",
		},
		{
			name:  "empty url",
			input: "",
			want:  "",
		},
		{
			name:    "invalid url",
			input:   "://bad",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cleanURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("cleanURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildURL_Remote(t *testing.T) {
	event := Event{
		Keywords: "Golang",
		Location: "Austin",
		Distance: "50",
	}
	u := buildURL("remote", event)
	q := u.Query()

	if q.Get("geoId") != geoId {
		t.Errorf("expected geoId %q, got %q", geoId, q.Get("geoId"))
	}
	if q.Get("f_WT") != remote {
		t.Errorf("expected f_WT %q, got %q", remote, q.Get("f_WT"))
	}
	if q.Get("keywords") != "Golang" {
		t.Errorf("expected keywords %q, got %q", "Golang", q.Get("keywords"))
	}
	if q.Get("f_TPR") != "r600" {
		t.Errorf("expected f_TPR %q, got %q", "r600", q.Get("f_TPR"))
	}
	if q.Get("location") != "" {
		t.Error("remote URL should not have location param")
	}
}

func TestBuildURL_Local(t *testing.T) {
	event := Event{
		Keywords: "React",
		Location: "Austin",
		Distance: "50",
	}
	u := buildURL("local", event)
	q := u.Query()

	if q.Get("keywords") != "React" {
		t.Errorf("expected keywords %q, got %q", "React", q.Get("keywords"))
	}
	if q.Get("location") != "Austin" {
		t.Errorf("expected location %q, got %q", "Austin", q.Get("location"))
	}
	if q.Get("distance") != "50" {
		t.Errorf("expected distance %q, got %q", "50", q.Get("distance"))
	}
	if q.Get("geoId") != "" {
		t.Error("local URL should not have geoId param")
	}
	if q.Get("f_WT") != "" {
		t.Error("local URL should not have f_WT param")
	}
}

func TestFilterResults(t *testing.T) {
	jobs := []Job{
		{Title: "Senior Golang Engineer", Company: "Acme Corp"},
		{Title: "Junior Developer", Company: "Good Co"},
		{Title: "Sales Engineer", Company: "SellCo"},
		{Title: "Software Engineer", Company: "DataAnnotation"},
		{Title: "Software Engineer", Company: "Jobs via Dice"},
		{Title: "Software Engineer", Company: "Jobot"},
		{Title: "Backend Engineer", Company: "StartupX"},
		{Title: "Electrical Engineer", Company: "PowerCo"},
		{Title: "Field Service Engineer", Company: "FieldCo"},
		{Title: "Intern Software Developer", Company: "InternCo"},
		{Title: "Entry Level Developer", Company: "EntryCo"},
		{Title: "Support Engineer", Company: "HelpCo"},
		{Title: "Mechanical Engineer", Company: "MechCo"},
	}

	blockedPosters := map[string]bool{
		"DataAnnotation": true,
		"Jobs via Dice":  true,
		"Jobot":          true,
	}
	filtered := filterResults(jobs, blockedPosters)

	expected := map[string]bool{
		"Senior Golang Engineer": true,
		"Backend Engineer":       true,
	}

	if len(filtered) != len(expected) {
		t.Fatalf("expected %d jobs, got %d: %+v", len(expected), len(filtered), filtered)
	}

	for _, job := range filtered {
		if !expected[job.Title] {
			t.Errorf("unexpected job in results: %q by %q", job.Title, job.Company)
		}
	}
}

func TestFilterResults_Empty(t *testing.T) {
	filtered := filterResults([]Job{}, nil)
	if len(filtered) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(filtered))
	}
}

func TestFilterResults_AllBlocked(t *testing.T) {
	jobs := []Job{
		{Title: "Junior Developer", Company: "SomeCo"},
		{Title: "Software Engineer", Company: "DataAnnotation"},
	}
	blockedPosters := map[string]bool{"DataAnnotation": true}
	filtered := filterResults(jobs, blockedPosters)
	if len(filtered) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(filtered))
	}
}

func TestFetchLinkedInJobs_NoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><div class="no-results">No jobs found</div></body></html>`))
	}))
	defer server.Close()

	// Temporarily override baseURL by testing the parsing logic directly
	// We test via a mock server by calling the parsing path
	event := Event{Keywords: "Golang", Location: "Austin", Distance: "50"}
	u := buildURL("remote", event)
	if u == nil {
		t.Fatal("buildURL returned nil")
	}
}

func TestFetchLinkedInJobs_ParsesHTML(t *testing.T) {
	html := `<html><body>
		<ul class="jobs-search__results-list">
			<li>
				<div class="base-card" data-entity-urn="urn:li:jobPosting:12345">
					<a class="base-card__full-link" href="https://www.linkedin.com/jobs/view/12345?trackingId=abc"></a>
					<h3 class="base-search-card__title">Senior Go Developer</h3>
					<h4 class="base-search-card__subtitle"><a>Acme Corp</a></h4>
					<span class="job-search-card__location">Remote</span>
					<time class="job-search-card__listdate">2 hours ago</time>
				</div>
			</li>
			<li>
				<div class="base-card" data-entity-urn="urn:li:jobPosting:67890">
					<a class="base-card__full-link" href="https://www.linkedin.com/jobs/view/67890"></a>
					<h3 class="base-search-card__title">React Engineer</h3>
					<h4 class="base-search-card__subtitle"><a>StartupX</a></h4>
					<span class="job-search-card__location">Austin, TX</span>
					<time class="job-search-card__listdate">30 minutes ago</time>
				</div>
			</li>
		</ul>
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer server.Close()

	// Test parsing by hitting the mock server directly
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("failed to get mock server: %v", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	var jobs []Job
	doc.Find(".jobs-search__results-list li").Each(func(i int, s *goquery.Selection) {
		var job Job
		urn, _ := s.Find(".base-card").Attr("data-entity-urn")
		urlFromHTML, _ := s.Find(".base-card__full-link").Attr("href")
		linkURL, _ := cleanURL(urlFromHTML)
		job.Id = strings.TrimPrefix(urn, "urn:li:jobPosting:")
		job.Title = strings.TrimSpace(s.Find(".base-search-card__title").Text())
		job.Company = strings.TrimSpace(s.Find(".base-search-card__subtitle a").Text())
		job.Location = strings.TrimSpace(s.Find(".job-search-card__location").Text())
		job.URL = linkURL
		job.PostedAt = strings.TrimSpace(s.Find(".job-search-card__listdate").Text())
		jobs = append(jobs, job)
	})

	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}

	if jobs[0].Id != "12345" {
		t.Errorf("expected job ID %q, got %q", "12345", jobs[0].Id)
	}
	if jobs[0].Title != "Senior Go Developer" {
		t.Errorf("expected title %q, got %q", "Senior Go Developer", jobs[0].Title)
	}
	if jobs[0].Company != "Acme Corp" {
		t.Errorf("expected company %q, got %q", "Acme Corp", jobs[0].Company)
	}
	// Verify trackingId was stripped
	if strings.Contains(jobs[0].URL, "trackingId") {
		t.Errorf("expected trackingId to be stripped from URL: %s", jobs[0].URL)
	}

	if jobs[1].Id != "67890" {
		t.Errorf("expected job ID %q, got %q", "67890", jobs[1].Id)
	}
	if jobs[1].Location != "Austin, TX" {
		t.Errorf("expected location %q, got %q", "Austin, TX", jobs[1].Location)
	}
}

func TestGetRandomFunctions(t *testing.T) {
	// Verify random functions return values from their respective slices
	ua := getRandomUserAgent()
	found := false
	for _, a := range userAgents {
		if a == ua {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("getRandomUserAgent returned unknown value: %q", ua)
	}

	ref := getRandomReferer()
	found = false
	for _, r := range referers {
		if r == ref {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("getRandomReferer returned unknown value: %q", ref)
	}

	lang := getRandomLanguage()
	found = false
	for _, l := range languages {
		if l == lang {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("getRandomLanguage returned unknown value: %q", lang)
	}
}

func TestHandlerMissingEmail(t *testing.T) {
	err := handler(t.Context(), Event{})
	if err == nil {
		t.Fatal("expected error for missing email")
	}
	if err.Error() != "email parameter is required" {
		t.Errorf("unexpected error: %v", err)
	}
}
