package httpapi

import _ "embed"

// openAPIDocument is the contract served at /openapi.yaml.
//
//go:embed openapi.yaml
var openAPIDocument string

// docsPage loads a pinned Scalar asset from jsDelivr. Interactive documentation
// therefore requires the browser to have external network access to that CDN.
const docsPage = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Garfex API reference</title>
</head>
<body>
  <p>Interactive API documentation requires external network access to the jsDelivr CDN.</p>
  <script id="api-reference" data-url="/openapi.yaml"></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.25.0"></script>
</body>
</html>
`
