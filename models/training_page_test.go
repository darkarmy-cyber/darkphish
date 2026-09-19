package models

import (
	"strings"
	"testing"

	"github.com/darkarmy-cyber/darkphish/internal/traininghtml"
	"gopkg.in/check.v1"
)

func TestStaticTrainingValidation(t *testing.T) {
	p := Page{Name: "Training", TrainingStatic: true, CaptureCredentials: true, CapturePasswords: true, RedirectURL: "https://example.test", HTML: `<form><input type=password value=SECRET></form><p>{{\broken}} Slovenské znaky</p>`}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if p.CaptureCredentials || p.CapturePasswords || p.RedirectURL != "" || !traininghtml.IsStatic(p.HTML) {
		t.Fatal("capture flags not disabled")
	}
	if strings.Contains(p.HTML, "SECRET") || !strings.Contains(p.HTML, `{{\broken}}`) {
		t.Fatal("unsafe or non-literal projection")
	}
	p.TrainingStatic = false
	if err := p.Validate(); err != nil || !p.TrainingStatic {
		t.Fatal("HTML marker did not preserve mode")
	}
}

func (s *ModelsSuite) TestStaticTrainingModeSurvivesEdits(c *check.C) {
	p := Page{Name: "Static training fixture", UserId: 1, TrainingStatic: true, HTML: `<p>{{\example}}</p><input type=password value=SECRET>`}
	c.Assert(PostPage(&p), check.IsNil)
	fetched, err := GetPage(p.Id, 1)
	c.Assert(err, check.IsNil)
	c.Assert(fetched.TrainingStatic, check.Equals, true)
	fetched.HTML = `<script>SECRET</script><form><input type=password value=SECRET></form><p>Edited</p>`
	fetched.TrainingStatic = false
	fetched.CapturePasswords = true
	fetched.CaptureCredentials = true
	c.Assert(PutPage(&fetched), check.IsNil)
	c.Assert(fetched.TrainingStatic, check.Equals, true)
	c.Assert(fetched.CapturePasswords, check.Equals, false)
	c.Assert(fetched.CaptureCredentials, check.Equals, false)
	c.Assert(strings.Contains(fetched.HTML, "SECRET"), check.Equals, false)
	all, err := GetPages(1)
	c.Assert(err, check.IsNil)
	c.Assert(len(all), check.Equals, 1)
	c.Assert(all[0].TrainingStatic, check.Equals, true)
}
