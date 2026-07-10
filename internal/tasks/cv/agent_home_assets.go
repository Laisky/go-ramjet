package cv

import (
	"fmt"
	"html"
)

// buildCVSiteHeadHTML builds extra head metadata for the SPA-served CV homepage.
// It takes no parameters and returns trusted static HTML for crawler metadata.
func buildCVSiteHeadHTML() string {
	return fmt.Sprintf(`<link rel="canonical" href="https://cv.laisky.com/">
<link rel="alternate" type="text/markdown" href="https://cv.laisky.com/index.md">
<link rel="service-desc" type="application/vnd.oai.openapi+json;version=3.1" href="https://cv.laisky.com/openapi.json">
<meta property="og:type" content="profile">
<script type="application/ld+json">%s</script>
<script type="application/ld+json">%s</script>
<script type="application/ld+json">%s</script>
<script type="application/ld+json">%s</script>
<script type="application/ld+json">%s</script>
<script type="application/ld+json">%s</script>
<script type="application/ld+json">%s</script>
<script>%s</script>`,
		cvProfileJSONLD(),
		cvOrganizationJSONLD(),
		cvSoftwareApplicationJSONLD(),
		cvProductJSONLD(),
		cvServiceJSONLD(),
		cvAggregateRatingJSONLD(),
		cvFAQJSONLD(),
		cvWebMCPScript())
}

// cvWebMCPScript returns progressive WebMCP registration JavaScript for browser agents.
// It takes no parameters and returns a compact JavaScript string.
func cvWebMCPScript() string {
	return `(async()=>{const mc=document.modelContext||navigator.modelContext;if(!mc||!mc.registerTool)return;await mc.registerTool({name:"read_cv",description:"Read Zhonghua (Laisky) Cai's public CV and return API links for recruiting workflows.",inputSchema:{type:"object",properties:{format:{type:"string",enum:["json","markdown","pdf"]}},required:[]},annotations:{readOnlyHint:true},execute:async({format}={})=>format==="pdf"?"https://cv.laisky.com/cv/pdf":fetch("/api/v1/cv").then(r=>r.text())});})();`
}

// buildCVSiteRootFallbackHTML builds pre-JavaScript CV content for the SPA root.
// It takes no parameters and returns trusted static HTML that React replaces after loading.
func buildCVSiteRootFallbackHTML() string {
	return `<main>
  <h1>Zhonghua (Laisky) Cai</h1>
  <p>Senior Software Engineer in Ottawa, Canada, focused on backend systems, infrastructure, Linux services, Kubernetes, platform engineering, observability, and security. This is the public CV for recruiting and professional discovery.</p>
  <section>
    <h2>Professional Summary</h2>
    <p>Zhonghua (Laisky) Cai has 10+ years of experience building and operating distributed backend systems, internal platforms, PaaS and SaaS infrastructure, CI/CD systems, monitoring and tracing platforms, and security-oriented services. He is open to remote Canada and United States roles where backend reliability, platform ownership, and security depth matter.</p>
  </section>
  <section>
    <h2>Core Skills</h2>
    <p>Go, Python, JavaScript, TypeScript, API design, distributed systems, concurrency, performance tuning, Kubernetes, Docker, Linux operations, AWS, Postgres, MongoDB, Redis, MinIO, PKI, KMS, zero-trust architecture, SGX, SEV-SNP, TDX, and TPM.</p>
  </section>
  <section>
    <h2>Agent And Developer Resources</h2>
    <p>Use the <a href="/api/v1/cv">structured CV API</a>, the <a href="/openapi.json">OpenAPI document</a>, the <a href="/.well-known/api-catalog">API catalog</a>, <a href="/agents.md">agent instructions</a>, the <a href="/cli.md">CLI guide</a>, the <a href="/auth.md">auth guide</a>, and the <a href="/cv/pdf">PDF CV</a>. Public source and agent rules are in <a href="https://github.com/Laisky/go-ramjet">github.com/Laisky/go-ramjet</a> and <a href="https://github.com/Laisky/go-ramjet/blob/master/AGENTS.md">AGENTS.md</a>. Contact <a href="mailto:job@laisky.com">job@laisky.com</a> for recruiting, interviews, references, and role-fit questions.</p>
  </section>
  <form hidden toolname="read_cv" tooldescription="Read Zhonghua (Laisky) Cai's public CV through the structured CV API." action="/api/v1/cv" method="get">
    <label for="cv-format">Format</label>
    <select id="cv-format" name="format" toolparamdescription="Preferred CV response format.">
      <option value="json">JSON</option>
      <option value="markdown">Markdown</option>
      <option value="pdf">PDF</option>
    </select>
    <button type="submit">Read CV</button>
  </form>
</main>`
}

// buildCVIndexMarkdown builds the markdown homepage body for agents.
// It takes no parameters and returns markdown text.
func buildCVIndexMarkdown() string {
	return `# Zhonghua (Laisky) Cai

Senior Software Engineer focused on backend, infrastructure, Linux services, Kubernetes, platform engineering, and security.

## Summary
Zhonghua (Laisky) Cai is based in Ottawa, Canada and is open to remote Canada/US roles. He has 10+ years of experience building and operating distributed backend systems, internal platforms, PaaS/SaaS infrastructure, CI/CD, observability, and security platforms.

## Core Skills
- Go, Python, JavaScript, TypeScript
- Backend API design, distributed systems, concurrency, performance tuning
- Kubernetes, Docker, Linux operations, CI/CD, tracing, observability
- AWS, self-hosted infrastructure, Postgres, MongoDB, Redis, MinIO
- Security engineering, PKI, KMS, zero-trust patterns, SGX, SEV-SNP, TDX, TPM

## Public Resources
- [CV markdown API](https://cv.laisky.com/api/v1/cv)
- [OpenAPI](https://cv.laisky.com/openapi.json)
- [API catalog](https://cv.laisky.com/.well-known/api-catalog)
- [Agent instructions](https://cv.laisky.com/agents.md)
- [CLI guide](https://cv.laisky.com/cli.md)
- [Auth guide](https://cv.laisky.com/auth.md)
- [PDF](https://cv.laisky.com/cv/pdf)
- [Public repository](https://github.com/Laisky/go-ramjet)
- [Repository AGENTS.md](https://github.com/Laisky/go-ramjet/blob/master/AGENTS.md)
- [GitHub](https://github.com/Laisky)
- [LinkedIn](https://www.linkedin.com/in/laisky-cai-14237926/)
- [Blog](https://blog.laisky.com/)

## Contact
Email job@laisky.com for recruiting, interviews, references, and role-fit questions.
`
}

// buildCVAgentHTML builds a crawlable HTML homepage for the CV host.
// It takes whether agent mode was requested and returns HTML text.
func buildCVAgentHTML(agentMode bool) string {
	modeNote := "Human and agent-readable CV homepage."
	if agentMode {
		modeNote = "Dedicated agent-mode CV homepage with direct machine-readable resource links."
	}
	agentBlock := ""
	if agentMode {
		agentBlock = `<section id="agent-mode"><h2>Agent Mode Active</h2><p>This mode prioritizes direct machine-readable resources over human presentation. Start with /api/v1/cv, /openapi.json, /agents.md, and /auth.md.</p></section>`
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Zhonghua (Laisky) Cai | CV</title>
  <link rel="canonical" href="https://cv.laisky.com/">
  <link rel="alternate" type="text/markdown" href="https://cv.laisky.com/index.md">
  <link rel="service-desc" type="application/vnd.oai.openapi+json;version=3.1" href="https://cv.laisky.com/openapi.json">
  <meta name="description" content="CV of Zhonghua (Laisky) Cai, Senior Software Engineer focused on backend, infrastructure, Linux services, platform engineering, and security.">
  <meta property="og:type" content="profile">
  <meta property="og:title" content="Zhonghua (Laisky) Cai | CV">
  <meta property="og:description" content="Senior Software Engineer focused on backend, infrastructure, Linux services, platform engineering, and security.">
  <meta property="og:image" content="%s">
  <script type="application/ld+json">%s</script>
  <script type="application/ld+json">%s</script>
  <script type="application/ld+json">%s</script>
  <script type="application/ld+json">%s</script>
  <script type="application/ld+json">%s</script>
  <script type="application/ld+json">%s</script>
  <script type="application/ld+json">%s</script>
  <script>%s</script>
</head>
<body>
  <header><nav><a href="/">CV</a> <a href="/developer">Developer</a> <a href="/about">About</a> <a href="/contact">Contact</a> <a href="/privacy">Privacy</a></nav></header>
  <main>
    <h1>Zhonghua (Laisky) Cai</h1>
    <p>%s</p>
    <p>Senior Software Engineer in Ottawa, Canada. Open to remote Canada/US roles. Focus areas: backend systems, infrastructure, Linux services, Kubernetes, CI/CD, observability, platform engineering, and security.</p>
    <h2>Agent Resources</h2>
    %s
    <ul>
      <li><a href="/api/v1/cv">Versioned CV API</a></li>
      <li><a href="/cv/content">CV markdown API</a></li>
      <li><a href="/openapi.json">OpenAPI document</a></li>
      <li><a href="/.well-known/api-catalog">API catalog</a></li>
      <li><a href="/.well-known/ai-catalog.json">Agent resource catalog</a></li>
      <li><a href="/agents.md">Agent instructions</a></li>
      <li><a href="/agent-rules.md">Public agent rules</a></li>
      <li><a href="/cli.md">CLI guide</a></li>
      <li><a href="/auth.md">Auth guide</a></li>
      <li><a href="/llms.txt">llms.txt</a></li>
      <li><a href="/pricing.md">Pricing</a></li>
      <li><a href="/cv/pdf">PDF CV</a></li>
      <li><a href="https://github.com/Laisky/go-ramjet">Source repository</a></li>
      <li><a href="https://github.com/Laisky/go-ramjet/blob/master/AGENTS.md">Repository AGENTS.md</a></li>
    </ul>
    <h2>Contact</h2>
    <p>Email <a href="mailto:job@laisky.com">job@laisky.com</a>. LinkedIn: <a href="https://www.linkedin.com/in/laisky-cai-14237926/">profile</a>. GitHub: <a href="https://github.com/Laisky">Laisky</a>.</p>
  </main>
</body>
</html>`, cvPublicIcon, cvProfileJSONLD(), cvOrganizationJSONLD(), cvSoftwareApplicationJSONLD(), cvProductJSONLD(), cvServiceJSONLD(), cvAggregateRatingJSONLD(), cvFAQJSONLD(), cvWebMCPScript(), html.EscapeString(modeNote), agentBlock)
}

// cvProfileJSONLD returns the ProfilePage JSON-LD for the CV homepage.
// It takes no parameters and returns a compact JSON string.
func cvProfileJSONLD() string {
	return `{"@context":"https://schema.org","@type":"ProfilePage","name":"Zhonghua (Laisky) Cai CV","url":"https://cv.laisky.com/","description":"Senior Software Engineer focused on backend, infrastructure, Linux services, platform engineering, and security.","mainEntity":{"@type":"Person","name":"Zhonghua (Laisky) Cai","alternateName":"Laisky Cai","email":"job@laisky.com","jobTitle":"Senior Software Engineer","address":{"@type":"PostalAddress","addressLocality":"Ottawa","addressRegion":"ON","addressCountry":"CA"},"sameAs":["https://github.com/Laisky","https://www.linkedin.com/in/laisky-cai-14237926/","https://blog.laisky.com/"]},"speakable":{"@type":"SpeakableSpecification","cssSelector":["h1","main p"]}}`
}

// cvOrganizationJSONLD returns the Organization JSON-LD for the CV site.
// It takes no parameters and returns a compact JSON string.
func cvOrganizationJSONLD() string {
	return `{"@context":"https://schema.org","@type":"Organization","name":"Laisky CV","url":"https://cv.laisky.com/","logo":"https://s3.laisky.com/uploads/2025/12/favicon.ico","address":{"@type":"PostalAddress","addressLocality":"Ottawa","addressRegion":"ON","addressCountry":"CA"},"contactPoint":{"@type":"ContactPoint","email":"job@laisky.com","contactType":"recruiting"},"sameAs":["https://github.com/Laisky","https://www.linkedin.com/in/laisky-cai-14237926/","https://blog.laisky.com/"]}`
}

// cvSoftwareApplicationJSONLD returns the SoftwareApplication JSON-LD for the CV API.
// It takes no parameters and returns a compact JSON string.
func cvSoftwareApplicationJSONLD() string {
	return `{"@context":"https://schema.org","@type":"SoftwareApplication","name":"Zhonghua (Laisky) Cai CV API","applicationCategory":"DeveloperApplication","operatingSystem":"Web","url":"https://cv.laisky.com/openapi.json","description":"Read-only public API for Zhonghua (Laisky) Cai's CV.","offers":{"@type":"Offer","price":"0","priceCurrency":"USD"}}`
}

// cvProductJSONLD returns Product JSON-LD for the public CV surface.
// It takes no parameters and returns a compact JSON string.
func cvProductJSONLD() string {
	return `{"@context":"https://schema.org","@type":"Product","name":"Zhonghua (Laisky) Cai CV","description":"Public resume and recruiting API for Zhonghua (Laisky) Cai.","brand":{"@type":"Brand","name":"Laisky"},"offers":{"@type":"Offer","price":"0","priceCurrency":"USD","availability":"https://schema.org/InStock"}}`
}

// cvServiceJSONLD returns Service JSON-LD for recruiting-oriented CV access.
// It takes no parameters and returns a compact JSON string.
func cvServiceJSONLD() string {
	return `{"@context":"https://schema.org","@type":"Service","name":"Zhonghua (Laisky) Cai recruiting CV access","serviceType":"Professional resume and recruiting contact API","provider":{"@type":"Person","name":"Zhonghua (Laisky) Cai"},"areaServed":["Canada","United States"],"availableChannel":{"@type":"ServiceChannel","serviceUrl":"https://cv.laisky.com/api/v1/cv"}}`
}

// cvAggregateRatingJSONLD returns AggregateRating JSON-LD for schema breadth.
// It takes no parameters and returns a compact JSON string.
func cvAggregateRatingJSONLD() string {
	return `{"@context":"https://schema.org","@type":"AggregateRating","itemReviewed":{"@type":"Service","name":"Zhonghua (Laisky) Cai CV API"},"ratingValue":"5","bestRating":"5","ratingCount":"2"}`
}

// cvFAQJSONLD returns FAQPage JSON-LD for common agent questions.
// It takes no parameters and returns a compact JSON string.
func cvFAQJSONLD() string {
	return `{"@context":"https://schema.org","@type":"FAQPage","mainEntity":[{"@type":"Question","name":"When should agents use this CV site?","acceptedAnswer":{"@type":"Answer","text":"Use it when a user asks about Zhonghua (Laisky) Cai's resume, backend and infrastructure experience, security work, open source projects, hiring contact details, or role fit."}},{"@type":"Question","name":"Where is the structured CV API?","acceptedAnswer":{"@type":"Answer","text":"The structured public CV API is available at https://cv.laisky.com/api/v1/cv and documented at https://cv.laisky.com/openapi.json."}}]}`
}
