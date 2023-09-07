package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
    "net/http"

	"github.com/chromedp/chromedp"
	"github.com/dgrijalva/jwt-go"
)

type JSFileInfo struct {
	URL          string
	Routes       []string
	Dependencies []string
	Tokens       []string
}

type Report struct {
	Domain         string
	JSFiles        []JSFileInfo
	TotalRoutes    int
	TotalDeps      int
	TotalTokens    int
	AllRoutes      []string
	AllDependencies []string
	AllTokens      []string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide a domain as a command-line argument.")
		os.Exit(1)
	}

	domain := os.Args[1]

	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	source, err := fetchPageSource(ctx, domain)
	if err != nil {
		log.Fatal(err)
	}

	jsFiles, err := findJSFiles(domain, source)
	if err != nil {
		log.Fatal(err)
	}

	report, err := generateReport(domain, jsFiles)
	if err != nil {
		log.Fatalf("Failed to generate report: %v", err)
	}

	htmlOutput := generateHTML(report)
	err = ioutil.WriteFile("output2.html", []byte(htmlOutput), 0644)
	if err != nil {
		log.Fatalf("Failed to write output2.html: %v", err)
	}

	fmt.Println("Results saved to output2.html")
}

func fetchPageSource(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ctx, cancel = chromedp.NewContext(ctx)
	defer cancel()

	var source string
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.OuterHTML(`html`, &source, chromedp.ByQuery),
	)
	if err != nil {
		return "", err
	}

	return source, nil
}

func findJSFiles(domain, source string) ([]string, error) {
	jsFilesRegex := regexp.MustCompile(`<script.*?src="(.*?)".*?></script>`)
	jsFilesMatches := jsFilesRegex.FindAllStringSubmatch(source, -1)

	var jsFiles []string
	for _, match := range jsFilesMatches {
		jsFile := match[1]
		if !strings.HasPrefix(jsFile, "http") && !strings.HasPrefix(jsFile, "//") {
			jsFile = domain + jsFile
		}
		jsFiles = append(jsFiles, jsFile)
	}

	return jsFiles, nil
}


func analyzeJSFile(url string) (*JSFileInfo, error) {
	if strings.HasPrefix(url, "//") {
		url = "https:" + url
	}

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	jsContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	routeRegex := regexp.MustCompile(`['"](/[^"'\s]+)['"]`)
	matches := routeRegex.FindAllStringSubmatch(string(jsContent), -1)
	routes := []string{}
	for _, match := range matches {
		route := match[1]

		invalidRoute := regexp.MustCompile(`[^\w\-/]+`)
		if invalidRoute.MatchString(route) {
			continue
		}

		routes = append(routes, route)
	}

	depRegex := regexp.MustCompile(`require\(["']([^"']+)["']\)`)
	deps := depRegex.FindAllStringSubmatch(string(jsContent), -1)
	dependencies := []string{}
	for _, dep := range deps {
		dependencies = append(dependencies, dep[1])
	}

	tokenRegex := regexp.MustCompile(`["']([a-zA-Z0-9\-_=]+\.[a-zA-Z0-9\-_=]+\.?[a-zA-Z0-9\-_+=/]*?)["']`)
	tokens := []string{}
	for _, match := range tokenRegex.FindAllStringSubmatch(string(jsContent), -1) {
		if jwtToken, err := jwt.Parse(strings.Trim(match[1], " '\""), func(token *jwt.Token) (interface{}, error) {
			return nil, nil
		}); err == nil && jwtToken.Valid {
			tokens = append(tokens, match[1])
		}
	}

	return &JSFileInfo{
		URL:          url,
		Routes:       routes,
		Dependencies: dependencies,
		Tokens:       tokens,
	}, nil
}

func generateHTML(report *Report) string {
	html := `<!doctype html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
	<title>Analysis Results</title>
	<link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0-alpha1/dist/css/bootstrap.min.css" rel="stylesheet">
	<link href="https://cdn.jsdelivr.net/npm/@tabler/core@1.0.0-alpha.7/dist/css/tabler.min.css" rel="stylesheet">
	<script src="https://cdn.jsdelivr.net/npm/@popperjs/core@2.11.6/dist/umd/popper.min.js"></script>
	<script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0-alpha1/dist/js/bootstrap.min.js"></script>
	<script>
		function showJSFileDetails(jsFileIndex) {
    var jsFileDetails = document.getElementsByClassName("js-file-details");
    for (var i = 0; i < jsFileDetails.length; i++) {
        if (jsFileIndex === "all") {
            jsFileDetails[i].style.display = i === 0 ? "block" : "none";
        } else {
            jsFileDetails[i].style.display = i === parseInt(jsFileIndex) ? "block" : "none";
        }
    }
}

	</script>
</head>
<body class="antialiased">
	<div class="container">
		<div class="page">
			<div class="page-content">
				<div class="card">
					<div class="card-header">
						<h1 class="card-title">Domain: %s</h1>
					</div>
					<div class="card-body">
						<table class="table table-striped">
							<thead>
								<tr>
									<th>Total JS Files</th>
									<th>Total Routes</th>
									<th>Total Dependencies</th>
									<th>Total Tokens</th>
								</tr>
							</thead>
							<tbody>
								<tr>
									<td>%d</td>
									<td>%d</td>
									<td>%d</td>
									<td>%d</td>
								</tr>
							</tbody>
						</table>
						<label for="js-file-selector">Select a JS file:</label>
						<select id="js-file-selector" class="form-select" onchange="showJSFileDetails(this.value)">
							<option value="all">All</option>
							%s
						</select>
						<div class="js-file-details-container mt-3" style="max-height: 400px; overflow-y: auto;">
							%s
						</div>
					</div>
				</div>
			</div>
		</div>
	</div>
</body>
</html>`
	
	jsFilesOptions := ""
	jsFilesDetails := ""

	for index, jsFileInfo := range report.JSFiles {
		jsFileOption := fmt.Sprintf(`<option value="%d">%s</option>`, index+1, jsFileInfo.URL)
		jsFilesOptions += jsFileOption

		jsFileDetail := fmt.Sprintf(`
		<div class="js-file-details" style="display: none;">
			<h2>JS: %s</h2>
			<h3>Routes:</h3>
			<ul>
				%s
			</ul>
			<h3>Dependencies:</h3>
			<ul>
				%s
			</ul>
			<h3>Tokens:</h3>
			<ul>
				%s
			</ul>
		</div>
		`, jsFileInfo.URL, generateListItems(jsFileInfo.Routes), generateListItems(jsFileInfo.Dependencies), generateListItems(jsFileInfo.Tokens))

		jsFilesDetails += jsFileDetail
	}

	allJsFilesDetails := fmt.Sprintf(`
	<div class="js-file-details" style="display: block;">
		<h2>All JS Files</h2>
		<h3>Routes:</h3>
		<ul>
			%s
		</ul>
		<h3>Dependencies:</h3>
		<ul>
			%s
		</ul>
		<h3>Tokens:</h3>
		<ul>
			%s
		</ul>
	</div>
	`, generateListItems(report.AllRoutes), generateListItems(report.AllDependencies), generateListItems(report.AllTokens))

	jsFilesDetails = allJsFilesDetails + jsFilesDetails

	return fmt.Sprintf(html, report.Domain, len(report.JSFiles), report.TotalRoutes, report.TotalDeps, report.TotalTokens, jsFilesOptions, jsFilesDetails)
}

func generateListItems(items []string) string {
	li := ""
	for _, item := range items {
		li += fmt.Sprintf("<li>%s</li>", item)
	}
	return li
}

func generateReport(domain string, jsFiles []string) (*Report, error) {
	report := &Report{
		Domain: domain,
	}

	for _, jsFile := range jsFiles {
		info, err := analyzeJSFile(jsFile)
		if err != nil {
			return nil, fmt.Errorf("Failed to analyze %s: %v", jsFile, err)
		}

		report.JSFiles = append(report.JSFiles, *info)
		report.TotalRoutes += len(info.Routes)
		report.TotalDeps += len(info.Dependencies)
		report.TotalTokens += len(info.Tokens)
		report.AllRoutes = append(report.AllRoutes, info.Routes...)
		report.AllDependencies = append(report.AllDependencies, info.Dependencies...)
		report.AllTokens = append(report.AllTokens, info.Tokens...)
	}

	return report, nil
}
