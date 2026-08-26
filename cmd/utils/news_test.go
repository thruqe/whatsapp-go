package cliutils

import (
	"strings"
	"testing"
)

const sampleArticleHTML = `<!DOCTYPE html>
<html>
<head>
    <meta property="og:title" content="Nigeria orders investigation into second fake government agency | AP News" />
    <meta property="og:image" content="https://dims.apnews.com/dims4/default/12345/image.jpg" />
</head>
<body>
    <h1 class="Page-headline">Nigeria orders investigation into second fake government agency</h1>
    <div class="RichTextStoryBody RichTextBody">
        <p>ABUJA, Nigeria (AP) — <span class="LinkEnhancement"><a class="Link AnClick-LinkEnhancement" data-gtm-enhancement-style="LinkEnhancementA" href="https://apnews.com/hub/nigeria">Nigeria’s</a></span> President Bola Tinubu on Friday ordered an investigation into a man accused of creating and running a fake government agency, the second such agency uncovered in the country in recent weeks.</p>
        <p>Musa Adamu Aliyu, the head of the country’s corruption watchdog, announced the investigation after briefing the president in the capital, Abuja.</p>
        <p>The latest bogus agency, called the National Brands Development and Made in Nigeria Special Project Office, was operating illegally from a government office and was headed by George Buchi Nwabueze, authorities said.</p>
        <p>The scandal is the latest involving an allegedly fake government agency under Tinubu’s administration. Last month, authorities discovered that another purported agency, the Presidential Foreign Investment Promotion Council, had operated for years, with its director meeting government officials and accessing public funds.</p>
        <div id="optimizelyHubpeekId" class="optimizelyHubpeekClass"></div>
        <div class="FreeStar Advertisement" data-module="">
            <div data-freestar-ad="__336x280 __336x280" id="" class="fs-feed-ad">
                <script data-cfasync="false" type="text/javascript">
                    freestar.queue.push(function () {});
                </script>
            </div>
        </div>
        <p>The man accused of creating the first bogus agency was arrested by police. Aliyu said the investigation into the first agency led to the discovery of the second.</p>
        <p>Analysts believe the perpetrators are working with senior government officials to use the agencies to siphon money from the national budget.</p>
        <p>Aliyu did not say whether the accused man had profited from the scheme before it was uncovered.</p>
        <p>Three senior government officials have been suspended, and authorities have issued an arrest warrant for Nwabueze.</p>
        <p>____</p>
        <p>AP’s Africa coverage at: <span class="LinkEnhancement"><a class="Link AnClick-LinkEnhancement" data-gtm-enhancement-style="LinkEnhancementA" href="https://apnews.com/hub/africa">https://apnews.com/hub/africa</a></span></p>
    </div>
</body>
</html>`

func TestParseAPNewsArticle(t *testing.T) {
	articleURL := "https://apnews.com/article/nigeria-fake-government-agency-bola-tinubu-5914f1c917b8dc309b3c6b71e78a1ea1"
	art, err := ParseAPNewsArticle(sampleArticleHTML, articleURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTitle := "Nigeria orders investigation into second fake government agency"
	if art.Title != expectedTitle {
		t.Errorf("expected Title %q, got %q", expectedTitle, art.Title)
	}

	if art.URL != articleURL {
		t.Errorf("expected URL %q, got %q", articleURL, art.URL)
	}

	if art.ImageURL != "https://dims.apnews.com/dims4/default/12345/image.jpg" {
		t.Errorf("expected ImageURL to be parsed, got %q", art.ImageURL)
	}

	if !strings.Contains(art.Content, "ABUJA, Nigeria (AP) — Nigeria’s President Bola Tinubu") {
		t.Errorf("expected content to contain first paragraph, got %q", art.Content)
	}

	if strings.Contains(art.Content, "freestar") || strings.Contains(art.Content, "optimizely") {
		t.Errorf("expected ads/scripts to be stripped, got %q", art.Content)
	}

	if !strings.Contains(art.Content, "Three senior government officials have been suspended") {
		t.Errorf("expected content to contain later paragraphs, got %q", art.Content)
	}
}

func TestParseAPNewsArticle_NoiseFiltering(t *testing.T) {
	rawHTML := `<!DOCTYPE html>
<html>
<body>
    <h1 class="Page-headline">Japanese lawmakers visit Beijing seeking to mend ties after fallout from Takaichi’s Taiwan remark</h1>
    <div class="Page-byline">
        <p>By MARI YAMAGUCHI</p>
        <p>Updated [hour]:[minute] [AMPM] [timezone], [monthFull] [day], [year]</p>
    </div>
    <!--
        Add AP News on Google
        Add AP News as your preferred source to see more of our stories on Google.
    -->
    <div class="SocialShare">
        <p>Share</p>
    </div>
    <div class="RichTextStoryBody RichTextBody">
        <p>TOKYO (AP) — A cross-party group of Japanese lawmakers headed to Beijing on Monday seeking to mend strained ties following Prime Minister Sanae Takaichi’s comment on Taiwan that angered China and led to te</p>
        <p>The delegation is scheduled to meet with senior Chinese officials during the three-day trip.</p>
    </div>
</body>
</html>`

	art, err := ParseAPNewsArticle(rawHTML, "https://apnews.com/article/japan-china-taiwan-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(art.Content, "MARI YAMAGUCHI") {
		t.Errorf("expected byline to be excluded, got %q", art.Content)
	}
	if strings.Contains(art.Content, "Updated") {
		t.Errorf("expected timestamp to be excluded, got %q", art.Content)
	}
	if strings.Contains(art.Content, "Add AP News") {
		t.Errorf("expected google promo to be excluded, got %q", art.Content)
	}
	if strings.Contains(art.Content, "Share") {
		t.Errorf("expected share to be excluded, got %q", art.Content)
	}
	if strings.Contains(art.Content, "-->") {
		t.Errorf("expected comments to be excluded, got %q", art.Content)
	}
	if !strings.HasPrefix(art.Content, "TOKYO (AP) — A cross-party group of Japanese lawmakers") {
		t.Errorf("expected content to start with story body dateline, got %q", art.Content)
	}
}

func TestParseAPNewsArticle_EmbeddedPromoExclusion(t *testing.T) {
	rawHTML := `<!DOCTYPE html>
<html>
<body>
    <div class="RichTextStoryBody RichTextBody">
        <p>A federal judge in New York has vacated a Trump administration policy that suspended the processing of visas from 75 countries, including Afghanistan, Iran, Russia and Somalia, whose nationals the Trump administration deemed likely to require public assistance in the United States.</p>
        <p>U.S. District Judge Jeannette Vargas, an appointee of former President Joe Biden, set aside the policy Friday as “contrary to law and in excess of statutory authority.”</p>
        <p>Secretary of State Marco Rubio exceeded his authority by issuing the policy, which “runs afoul” of the Immigration and Nationality Act by mandating “the refusal of visas to eligible applicants without any basis in law,” the judge ruled.</p>
        <p>Vargas said the policy also undermines the congressional requirement that puts consular officers at the forefront of any visa decision.</p>
        <p>“Catholic social teaching calls us to uphold the dignity of every person and recognize the family as the foundation of society,” she said in a statement. “This decision affirms both those values and the rule of law, allowing families to once again move forward toward reunification.”</p>
        <div class="Enhancement">
            <div class="PagePromo">
                <p>AP Decision Notes: What to expect in South Carolina’s special U.S. Senate primary runoff</p>
                <div class="PagePromo-description">
                    <span>Follow the key races in South Carolina</span>
                </div>
            </div>
        </div>
        <aside class="InlineCard">
            <p>RELATED: Supreme Court to hear arguments on immigration enforcement</p>
        </aside>
    </div>
</body>
</html>`

	art, err := ParseAPNewsArticle(rawHTML, "https://apnews.com/article/trump-visa-policy-ruling")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(art.Content, "AP Decision Notes") {
		t.Errorf("expected AP Decision Notes to be excluded, got %q", art.Content)
	}
	if strings.Contains(art.Content, "RELATED:") {
		t.Errorf("expected RELATED promo to be excluded, got %q", art.Content)
	}
	if strings.Contains(art.Content, "\n\n\n") {
		t.Errorf("expected no redundant multi-newlines, got %q", art.Content)
	}
	if !strings.HasSuffix(art.Content, "allowing families to once again move forward toward reunification.”") {
		t.Errorf("expected content to end cleanly with last story paragraph, got %q", art.Content)
	}
}

func TestExtractArticleURLsFromNewsText(t *testing.T) {
	newsListText := `AP News - Nigeria

1. Nigeria orders investigation into second fake government agency
   President Bola Tinubu on Friday ordered an investigation...
   https://apnews.com/article/nigeria-fake-government-agency-bola-tinubu-5914f1c917b8dc309b3c6b71e78a1ea1

2. Fuel tanker explosion in northern Nigeria kills at least 140
   A gasoline tanker crashed and exploded in northern Nigeria...
   https://apnews.com/article/nigeria-fuel-tanker-explosion-jigawa-death-toll-84930129381203

3. Nigeria secures release of 20 medical students
   Police in Nigeria have rescued all 20 medical students...
   https://apnews.com/article/nigeria-kidnapped-students-freed-rescue-operation-192837461`

	urls := ExtractAPNewsArticleURLs(newsListText)
	if len(urls) != 3 {
		t.Fatalf("expected 3 URLs, got %d: %v", len(urls), urls)
	}

	if urls[0] != "https://apnews.com/article/nigeria-fake-government-agency-bola-tinubu-5914f1c917b8dc309b3c6b71e78a1ea1" {
		t.Errorf("expected first url match, got %q", urls[0])
	}
	if urls[1] != "https://apnews.com/article/nigeria-fuel-tanker-explosion-jigawa-death-toll-84930129381203" {
		t.Errorf("expected second url match, got %q", urls[1])
	}
	if urls[2] != "https://apnews.com/article/nigeria-kidnapped-students-freed-rescue-operation-192837461" {
		t.Errorf("expected third url match, got %q", urls[2])
	}
}

func TestParseNewsSelectionNumber(t *testing.T) {
	tests := []struct {
		input       string
		expectedNum int
		expectedOk  bool
	}{
		{"1", 1, true},
		{"2", 2, true},
		{"5", 5, true},
		{"#1", 1, true},
		{"#3", 3, true},
		{"1.", 1, true},
		{"2.", 2, true},
		{"article 2", 2, true},
		{"news 3", 3, true},
		{"read 1", 1, true},
		{"show 4", 4, true},
		{".news 1", 1, true},
		{"!news 2", 2, true},
		{"hello", 0, false},
		{"123abc", 0, false},
		{"", 0, false},
	}

	for _, tt := range tests {
		num, ok := ParseNewsSelectionNumber(tt.input)
		if ok != tt.expectedOk {
			t.Errorf("ParseNewsSelectionNumber(%q) ok = %v, expected %v", tt.input, ok, tt.expectedOk)
		}
		if num != tt.expectedNum {
			t.Errorf("ParseNewsSelectionNumber(%q) num = %d, expected %d", tt.input, num, tt.expectedNum)
		}
	}
}
