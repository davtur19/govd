package instagram

import (
	"fmt"
	"io"
	"math/big"
	"net/url"
	"regexp"
	"strings"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/bytedance/sonic"

	"github.com/govdbot/govd/internal/config"
	"github.com/govdbot/govd/internal/database"
	"github.com/govdbot/govd/internal/models"
)

const (
	modernIGBaseURL = "https://www.instagram.com/"

	modernGraphQLEndpoint = "https://www.instagram.com/api/graphql"
	modernPolarisAction   = "PolarisLoggedOutDesktopWWWPostRootContentQuery"
	modernGraphQLDocID    = "27130156389949648"

	modernIGAppID = "936619743392459"
	modernASBDID  = "359341"

	modernUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) " +
		"Chrome/146.0.0.0 Safari/537.36"
)

var modernLSDPattern = regexp.MustCompile(
	`\["LSD",\[\],\{"token":"([^"]+)"`,
)

type modernGraphQLResponse struct {
	Data *struct {
		Media *struct {
			LoggedOut *modernProductMedia `json:"if_not_gated_logged_out"`
		} `json:"xig_polaris_media"`
	} `json:"data"`
}

type modernProductMedia struct {
	PK string `json:"pk"`

	Caption *struct {
		Text string `json:"text"`
	} `json:"caption"`

	VideoVersions []modernVideoVersion `json:"video_versions"`

	ImageVersions2 struct {
		Candidates []modernImageCandidate `json:"candidates"`
	} `json:"image_versions2"`

	CarouselMedia []*modernProductMedia `json:"carousel_media"`
}

type modernVideoVersion struct {
	URL    string `json:"url"`
	Width  int32  `json:"width"`
	Height int32  `json:"height"`
}

type modernImageCandidate struct {
	URL    string `json:"url"`
	Width  int32  `json:"width"`
	Height int32  `json:"height"`
}

func shortcodeToMediaID(shortcode string) (string, error) {
	// Same conversion used by Instagram clients:
	// ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

	// Instagram can append 28 chars to private-post shortcodes.
	if len(shortcode) > 28 {
		shortcode = shortcode[:len(shortcode)-28]
	}

	if shortcode == "" {
		return "", fmt.Errorf("empty Instagram shortcode")
	}

	base := big.NewInt(64)
	value := big.NewInt(0)

	for _, c := range shortcode {
		idx := strings.IndexRune(alphabet, c)
		if idx < 0 {
			return "", fmt.Errorf("invalid Instagram shortcode character %q", c)
		}

		value.Mul(value, base)
		value.Add(value, big.NewInt(int64(idx)))
	}

	return value.String(), nil
}

func newModernInstagramClient() (tls_client.HttpClient, error) {
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithClientProfile(profiles.Chrome_146),
		tls_client.WithCookieJar(tls_client.NewCookieJar()),
	}

	// GOVD already loads PROXY=socks5://wireproxy:1080 into config.Env.Proxy.
	if config.Env.Proxy != "" {
		options = append(options, tls_client.WithProxyUrl(config.Env.Proxy))
	}

	return tls_client.NewHttpClient(
		tls_client.NewNoopLogger(),
		options...,
	)
}

func setModernBrowserHeaders(req *http.Request) {
	req.Header.Set(
		"sec-ch-ua",
		`"Not:A-Brand";v="99", "Google Chrome";v="146", "Chromium";v="146"`,
	)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)

	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("User-Agent", modernUserAgent)
	req.Header.Set(
		"Accept",
		"text/html,application/xhtml+xml,application/xml;q=0.9,"+
			"image/avif,image/webp,image/apng,*/*;q=0.8,"+
			"application/signed-exchange;v=b3;q=0.7",
	)
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Priority", "u=0, i")

	req.Header[http.HeaderOrderKey] = []string{
		"sec-ch-ua",
		"sec-ch-ua-mobile",
		"sec-ch-ua-platform",
		"upgrade-insecure-requests",
		"user-agent",
		"accept",
		"sec-fetch-site",
		"sec-fetch-mode",
		"sec-fetch-user",
		"sec-fetch-dest",
		"accept-language",
		"priority",
		"cookie",
	}
}

func setModernAPIHeaders(req *http.Request) {
	setModernBrowserHeaders(req)

	req.Header.Del("Upgrade-Insecure-Requests")
	req.Header.Del("Sec-Fetch-User")

	req.Header.Set("Accept", "*/*")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Priority", "u=1, i")

	req.Header.Set("X-IG-App-ID", modernIGAppID)
	req.Header.Set("X-ASBD-ID", modernASBDID)
	req.Header.Set("X-IG-WWW-Claim", "0")
	req.Header.Set("Origin", "https://www.instagram.com")

	req.Header[http.HeaderOrderKey] = []string{
		"content-length",
		"sec-ch-ua-platform",
		"x-csrftoken",
		"x-fb-friendly-name",
		"x-fb-lsd",
		"x-ig-app-id",
		"x-asbd-id",
		"x-ig-www-claim",
		"sec-ch-ua",
		"sec-ch-ua-mobile",
		"user-agent",
		"content-type",
		"x-requested-with",
		"accept",
		"origin",
		"sec-fetch-site",
		"sec-fetch-mode",
		"sec-fetch-dest",
		"referer",
		"accept-language",
		"priority",
		"cookie",
	}
}

func getModernLSD(client tls_client.HttpClient) (string, error) {
	req, err := http.NewRequest(http.MethodGet, modernIGBaseURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build Instagram session request: %w", err)
	}

	setModernBrowserHeaders(req)
	req.Header.Set(
		"Accept",
		"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
	)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to initialize Instagram session: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read Instagram session page: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Instagram session returned %s", resp.Status)
	}

	match := modernLSDPattern.FindSubmatch(body)
	if len(match) != 2 {
		return "", fmt.Errorf("Instagram LSD token not found")
	}

	return string(match[1]), nil
}

func modernAccessibilityCheck(
	client tls_client.HttpClient,
	mediaID string,
) string {
	checkURL := modernIGBaseURL +
		"api/v1/web/get_ruling_for_content/?" +
		"content_type=MEDIA&target_id=" +
		url.QueryEscape(mediaID)

	req, err := http.NewRequest(http.MethodGet, checkURL, nil)
	if err != nil {
		return ""
	}

	setModernAPIHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}

	var ruling struct {
		Status string `json:"status"`
	}

	if err := sonic.ConfigFastest.Unmarshal(body, &ruling); err != nil {
		return ""
	}

	// yt-dlp only trusts the CSRF cookie when Instagram explicitly
	// grants access to the media.
	if ruling.Status != "ok" {
		return ""
	}

	instagramURL, err := url.Parse(modernIGBaseURL)
	if err != nil {
		return ""
	}

	for _, cookie := range client.GetCookies(instagramURL) {
		if cookie.Name == "csrftoken" && cookie.Value != "" {
			return cookie.Value
		}
	}

	return ""
}

func getModernProduct(
	ctx *models.ExtractorContext,
) (*modernProductMedia, error) {
	mediaID, err := shortcodeToMediaID(ctx.ContentID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert Instagram shortcode: %w", err)
	}

	client, err := newModernInstagramClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Instagram TLS client: %w", err)
	}
	defer client.CloseIdleConnections()

	lsd, err := getModernLSD(client)
	if err != nil {
		return nil, err
	}

	// Mirrors Instagram's current anonymous extraction flow.
	// This request is allowed to fail; it can still seed useful session cookies.
	csrfToken := modernAccessibilityCheck(client, mediaID)

	variables, err := sonic.ConfigFastest.Marshal(map[string]string{
		"media_id": mediaID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Instagram GraphQL variables: %w", err)
	}

	form := url.Values{}
	form.Set("lsd", lsd)
	form.Set("fb_api_caller_class", "RelayModern")
	form.Set("fb_api_req_friendly_name", modernPolarisAction)
	form.Set("server_timestamps", "true")
	form.Set("variables", string(variables))
	form.Set("doc_id", modernGraphQLDocID)

	req, err := http.NewRequest(
		http.MethodPost,
		modernGraphQLEndpoint,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build Instagram GraphQL request: %w", err)
	}

	setModernAPIHeaders(req)

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-FB-Friendly-Name", modernPolarisAction)
	req.Header.Set("X-FB-LSD", lsd)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if csrfToken != "" {
		req.Header.Set("X-CSRFToken", csrfToken)
	}
	req.Header.Set("Referer", ctx.ContentURL)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Instagram GraphQL request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Instagram GraphQL response: %w", err)
	}

	var graphErr error

	if resp.StatusCode == http.StatusOK {
		var gql modernGraphQLResponse

		if err := sonic.ConfigFastest.Unmarshal(body, &gql); err == nil {
			if gql.Data != nil &&
				gql.Data.Media != nil &&
				gql.Data.Media.LoggedOut != nil {
				return gql.Data.Media.LoggedOut, nil
			}

			graphErr = fmt.Errorf(
				"modern Instagram GraphQL returned no logged-out media",
			)
		} else {
			finalURL := ""
			if resp.Request != nil && resp.Request.URL != nil {
				finalURL = resp.Request.URL.String()
			}

			graphErr = fmt.Errorf(
				"failed to decode modern Instagram GraphQL response "+
					"(final_url=%s content_type=%q): %w",
				finalURL,
				resp.Header.Get("Content-Type"),
				err,
			)
		}

		// Instagram sometimes returns an HTML page from /api/graphql.
		// Try to extract Relay data directly from that HTML first.
		if product, err := parseModernProductFromHTML(body); err == nil {
			return product, nil
		}
	} else {
		graphErr = fmt.Errorf(
			"modern Instagram GraphQL returned %s",
			resp.Status,
		)
	}

	// Final modern fallback: fetch /p/<shortcode> using the same
	// impersonated TLS client and look for xig_polaris_media in data-sjs.
	product, pageErr := getModernProductFromPostPage(client, ctx)
	if pageErr == nil {
		return product, nil
	}

	return nil, fmt.Errorf(
		"GraphQL failed (csrf=%t): %v; post page fallback failed: %w",
		csrfToken != "",
		graphErr,
		pageErr,
	)
}

func bestModernImage(product *modernProductMedia) *modernImageCandidate {
	var best *modernImageCandidate
	var bestArea int64

	for i := range product.ImageVersions2.Candidates {
		candidate := &product.ImageVersions2.Candidates[i]

		if candidate.URL == "" {
			continue
		}

		area := int64(candidate.Width) * int64(candidate.Height)

		if best == nil || area > bestArea {
			best = candidate
			bestArea = area
		}
	}

	return best
}

func addModernProduct(
	media *models.Media,
	product *modernProductMedia,
) error {
	if product == nil {
		return fmt.Errorf("nil Instagram product")
	}

	thumbnail := bestModernImage(product)

	// Video / Reel
	if len(product.VideoVersions) > 0 {
		item := media.NewItem()
		added := false

		for i, version := range product.VideoVersions {
			if version.URL == "" {
				continue
			}

			var thumbnails []string
			if thumbnail != nil && thumbnail.URL != "" {
				thumbnails = []string{thumbnail.URL}
			}

			item.AddFormats(&models.MediaFormat{
				FormatID:     fmt.Sprintf("video-%d", i+1),
				Type:         database.MediaTypeVideo,
				VideoCodec:   database.MediaCodecAvc,
				AudioCodec:   database.MediaCodecAac,
				URL:          []string{version.URL},
				ThumbnailURL: thumbnails,
				Width:        version.Width,
				Height:       version.Height,
			})

			added = true
		}

		if !added {
			return fmt.Errorf("Instagram video_versions contains no usable URL")
		}

		return nil
	}

	// Photo
	if thumbnail != nil && thumbnail.URL != "" {
		item := media.NewItem()

		item.AddFormats(&models.MediaFormat{
			FormatID: "image",
			Type:     database.MediaTypePhoto,
			URL:      []string{thumbnail.URL},
			Width:    thumbnail.Width,
			Height:   thumbnail.Height,
		})

		return nil
	}

	return fmt.Errorf("Instagram product contains neither video nor image")
}

func GetModernGQLMedia(
	ctx *models.ExtractorContext,
) (*models.Media, error) {
	product, err := getModernProduct(ctx)
	if err != nil {
		return nil, err
	}

	media := ctx.NewMedia()

	if product.Caption != nil {
		media.SetCaption(product.Caption.Text)
	}

	if len(product.CarouselMedia) > 0 {
		for i, child := range product.CarouselMedia {
			if err := addModernProduct(media, child); err != nil {
				return nil, fmt.Errorf(
					"failed to parse Instagram carousel item %d: %w",
					i,
					err,
				)
			}
		}

		return media, nil
	}

	if err := addModernProduct(media, product); err != nil {
		return nil, err
	}

	return media, nil
}

var modernSJSBlockPattern = regexp.MustCompile(
	`(?is)<script\b[^>]*\bdata-sjs(?:=["'][^"']*["'])?[^>]*>(.*?)</script>`,
)

func findModernProductInJSON(v any) *modernProductMedia {
	switch node := v.(type) {
	case map[string]any:
		if rawMedia, ok := node["xig_polaris_media"]; ok {
			if mediaMap, ok := rawMedia.(map[string]any); ok {
				if rawProduct, ok := mediaMap["if_not_gated_logged_out"]; ok && rawProduct != nil {
					buf, err := sonic.ConfigFastest.Marshal(rawProduct)
					if err == nil {
						var product modernProductMedia
						if sonic.ConfigFastest.Unmarshal(buf, &product) == nil {
							return &product
						}
					}
				}
			}
		}

		for _, child := range node {
			if product := findModernProductInJSON(child); product != nil {
				return product
			}
		}

	case []any:
		for _, child := range node {
			if product := findModernProductInJSON(child); product != nil {
				return product
			}
		}
	}

	return nil
}

func parseModernProductFromHTML(body []byte) (*modernProductMedia, error) {
	matches := modernSJSBlockPattern.FindAllSubmatch(body, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		var root any
		if err := sonic.ConfigFastest.Unmarshal(match[1], &root); err != nil {
			continue
		}

		if product := findModernProductInJSON(root); product != nil {
			return product, nil
		}
	}

	return nil, fmt.Errorf(
		"xig_polaris_media not found in %d data-sjs blocks",
		len(matches),
	)
}

func getModernProductFromPostPage(
	client tls_client.HttpClient,
	ctx *models.ExtractorContext,
) (*modernProductMedia, error) {
	pageURL := modernIGBaseURL + "p/" + url.PathEscape(ctx.ContentID) + "/"

	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build Instagram post page request: %w", err)
	}

	setModernBrowserHeaders(req)
	req.Header.Set(
		"Accept",
		"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
	)
	req.Header.Set("Referer", modernIGBaseURL)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Instagram post page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Instagram post page: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Instagram post page returned %s", resp.Status)
	}

	product, err := parseModernProductFromHTML(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Instagram post page: %w", err)
	}

	return product, nil
}
