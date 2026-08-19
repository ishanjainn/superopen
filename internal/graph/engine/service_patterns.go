package engine

import (
	"path"
	"strings"
)

type serviceKind uint8

const (
	serviceNone serviceKind = iota
	serviceHTTP
	serviceAsync
	serviceConfig
	serviceRouteRegistration
)

type servicePattern struct {
	identity string
	kind     serviceKind
	broker   string
}

// These identities are the provider-neutral port of the Superopen asset
// service allowlists. Matching remains case-sensitive and substring based.
var servicePatterns = []servicePattern{
	// Route registration is intentionally first.
	{"gin-gonic/gin", serviceRouteRegistration, ""}, {"gin.", serviceRouteRegistration, ""},
	{"go-chi/chi", serviceRouteRegistration, ""}, {"chi.", serviceRouteRegistration, ""},
	{"gorilla/mux", serviceRouteRegistration, ""}, {"labstack/echo", serviceRouteRegistration, ""},
	{"echo.", serviceRouteRegistration, ""}, {"gofiber/fiber", serviceRouteRegistration, ""},
	{"fiber.", serviceRouteRegistration, ""}, {"net/http.ServeMux", serviceRouteRegistration, ""},
	{"http.ServeMux", serviceRouteRegistration, ""}, {"httprouter", serviceRouteRegistration, ""},
	{"express", serviceRouteRegistration, ""}, {"fastify", serviceRouteRegistration, ""},
	{"koa-router", serviceRouteRegistration, ""}, {"hono", serviceRouteRegistration, ""},
	{"hapi", serviceRouteRegistration, ""}, {"flask", serviceRouteRegistration, ""},
	{"FastAPI", serviceRouteRegistration, ""}, {"starlette", serviceRouteRegistration, ""},
	{"Laravel", serviceRouteRegistration, ""}, {"Illuminate.Routing", serviceRouteRegistration, ""},
	{"Symfony.Routing", serviceRouteRegistration, ""}, {"ktor.server", serviceRouteRegistration, ""},
	{"ktor.routing", serviceRouteRegistration, ""}, {"actix-web", serviceRouteRegistration, ""},
	{"actix_web", serviceRouteRegistration, ""}, {"axum", serviceRouteRegistration, ""},
	{"rocket", serviceRouteRegistration, ""}, {"Spring", serviceRouteRegistration, ""},
	{"jakarta.ws.rs", serviceRouteRegistration, ""}, {"Microsoft.AspNetCore", serviceRouteRegistration, ""},
	{"MapGet", serviceRouteRegistration, ""}, {"MapPost", serviceRouteRegistration, ""},
	{"ActionDispatch", serviceRouteRegistration, ""}, {"Sinatra", serviceRouteRegistration, ""},
	{"Phoenix.Router", serviceRouteRegistration, ""}, {"akka.http.scaladsl.server", serviceRouteRegistration, ""},
	{"play.api.routing", serviceRouteRegistration, ""},

	{"requests", serviceHTTP, ""}, {"httpx", serviceHTTP, ""}, {"aiohttp", serviceHTTP, ""},
	{"urllib", serviceHTTP, ""}, {"urllib3", serviceHTTP, ""}, {"httplib2", serviceHTTP, ""},
	{"pycurl", serviceHTTP, ""}, {"treq", serviceHTTP, ""}, {"uplink", serviceHTTP, ""},
	{"axios", serviceHTTP, ""}, {"superagent", serviceHTTP, ""}, {"needle", serviceHTTP, ""},
	{"node-fetch", serviceHTTP, ""}, {"undici", serviceHTTP, ""}, {"ofetch", serviceHTTP, ""},
	{"wretch", serviceHTTP, ""}, {"sindresorhus/ky", serviceHTTP, ""}, {"phin", serviceHTTP, ""},
	{"net/http", serviceHTTP, ""}, {"resty", serviceHTTP, ""}, {"sling", serviceHTTP, ""},
	{"heimdall", serviceHTTP, ""}, {"gentleman", serviceHTTP, ""}, {"retryablehttp", serviceHTTP, ""},
	{"HttpClient", serviceHTTP, ""}, {"OkHttp", serviceHTTP, ""}, {"okhttp3", serviceHTTP, ""},
	{"RestTemplate", serviceHTTP, ""}, {"WebClient", serviceHTTP, ""}, {"Unirest", serviceHTTP, ""},
	{"AsyncHttpClient", serviceHTTP, ""}, {"apache.http", serviceHTTP, ""}, {"Retrofit", serviceHTTP, ""},
	{"Feign", serviceHTTP, ""}, {"ktor.client", serviceHTTP, ""}, {"kittinunf.fuel", serviceHTTP, ""},
	{"reqwest", serviceHTTP, ""}, {"hyper", serviceHTTP, ""}, {"surf", serviceHTTP, ""},
	{"ureq", serviceHTTP, ""}, {"isahc", serviceHTTP, ""}, {"attohttpc", serviceHTTP, ""},
	{"RestSharp", serviceHTTP, ""}, {"Flurl", serviceHTTP, ""}, {"Refit", serviceHTTP, ""},
	{"HTTParty", serviceHTTP, ""}, {"Faraday", serviceHTTP, ""}, {"RestClient", serviceHTTP, ""},
	{"Typhoeus", serviceHTTP, ""}, {"Excon", serviceHTTP, ""}, {"Net::HTTP", serviceHTTP, ""},
	{"Guzzle", serviceHTTP, ""}, {"guzzle", serviceHTTP, ""}, {"curl", serviceHTTP, ""},
	{"Symfony\\HttpClient", serviceHTTP, ""}, {"cpr", serviceHTTP, ""}, {"cpp-httplib", serviceHTTP, ""},
	{"Poco.Net", serviceHTTP, ""}, {"Beast", serviceHTTP, ""}, {"Alamofire", serviceHTTP, ""},
	{"Moya", serviceHTTP, ""}, {"URLSession", serviceHTTP, ""}, {"Dio", serviceHTTP, ""},
	{"dio", serviceHTTP, ""}, {"package:http", serviceHTTP, ""}, {"Chopper", serviceHTTP, ""},
	{"HTTPoison", serviceHTTP, ""}, {"Tesla", serviceHTTP, ""}, {"Finch", serviceHTTP, ""},
	{"Mint.HTTP", serviceHTTP, ""}, {"sttp", serviceHTTP, ""}, {"akka.http", serviceHTTP, ""},
	{"http4s", serviceHTTP, ""}, {"scalaj", serviceHTTP, ""}, {"wreq", serviceHTTP, ""},
	{"http-client", serviceHTTP, ""}, {"http-conduit", serviceHTTP, ""}, {"servant-client", serviceHTTP, ""},
	{"Network.HTTP", serviceHTTP, ""}, {"socket.http", serviceHTTP, ""}, {"resty.http", serviceHTTP, ""},

	{"cloudtasks", serviceAsync, "cloud_tasks"}, {"cloud_tasks", serviceAsync, "cloud_tasks"},
	{"cloud.tasks", serviceAsync, "cloud_tasks"}, {"CloudTasks", serviceAsync, "cloud_tasks"},
	{"pubsub", serviceAsync, "pubsub"}, {"cloud.pubsub", serviceAsync, "pubsub"},
	{"aws-sdk-go/service/sqs", serviceAsync, "sqs"}, {"aws-sdk-go.service.sqs", serviceAsync, "sqs"},
	{"Amazon.SQS", serviceAsync, "sqs"}, {"@aws-sdk/client-sqs", serviceAsync, "sqs"},
	{"aws-sdk-go/service/sns", serviceAsync, "sns"}, {"Amazon.SNS", serviceAsync, "sns"},
	{"@aws-sdk/client-sns", serviceAsync, "sns"}, {"eventbridge", serviceAsync, "eventbridge"},
	{"ServiceBus", serviceAsync, "servicebus"}, {"Azure.Messaging", serviceAsync, "servicebus"},
	{"kafka", serviceAsync, "kafka"}, {"Kafka", serviceAsync, "kafka"}, {"kafkajs", serviceAsync, "kafka"},
	{"sarama", serviceAsync, "kafka"}, {"rdkafka", serviceAsync, "kafka"}, {"confluent", serviceAsync, "kafka"},
	{"amqp", serviceAsync, "rabbitmq"}, {"AMQP", serviceAsync, "rabbitmq"},
	{"amqplib", serviceAsync, "rabbitmq"}, {"RabbitMQ", serviceAsync, "rabbitmq"},
	{"nats", serviceAsync, "nats"}, {"NATS", serviceAsync, "nats"}, {"celery", serviceAsync, "celery"},
	{"dramatiq", serviceAsync, "dramatiq"}, {"bullmq", serviceAsync, "bullmq"},
	{"Sidekiq", serviceAsync, "sidekiq"}, {"temporalio", serviceAsync, "temporal"},
	{"@temporalio", serviceAsync, "temporal"}, {"mqtt", serviceAsync, "mqtt"},
	{"paho.mqtt", serviceAsync, "mqtt"}, {"dapr.clients.grpc", serviceAsync, "dapr"},

	{"getenv", serviceConfig, ""}, {"Getenv", serviceConfig, ""}, {"getEnv", serviceConfig, ""},
	{"LookupEnv", serviceConfig, ""}, {"lookupEnv", serviceConfig, ""}, {"get_env", serviceConfig, ""},
	{"fetch_env", serviceConfig, ""}, {"GetEnvironmentVariable", serviceConfig, ""},
	{"getProperty", serviceConfig, ""}, {"getEnvironment", serviceConfig, ""},
	{"viper", serviceConfig, ""}, {"envconfig", serviceConfig, ""}, {"godotenv", serviceConfig, ""},
	{"decouple", serviceConfig, ""}, {"dynaconf", serviceConfig, ""}, {"dotenv", serviceConfig, ""},
	{"nconf", serviceConfig, ""}, {"convict", serviceConfig, ""}, {"envalid", serviceConfig, ""},
}

func matchService(identity string) (serviceKind, string) {
	for _, pattern := range servicePatterns {
		if serviceIdentityMatch(identity, pattern.identity) {
			return pattern.kind, pattern.broker
		}
	}
	return serviceNone, ""
}

// serviceIdentityMatch matches Superopen strstr matching, but rejects bare
// alphanumeric tokens embedded inside larger identifiers (e.g. "dio" in
// "audioUrl" / "_parseAudioArgs"). Punctuated identities like "net/http"
// keep plain substring matching.
func serviceIdentityMatch(identity, pattern string) bool {
	if pattern == "" || !strings.Contains(identity, pattern) {
		return false
	}
	if serviceIdentityNeedsBoundary(pattern) {
		return containsWithIdentBoundary(identity, pattern)
	}
	return true
}

func serviceIdentityNeedsBoundary(pattern string) bool {
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' {
			continue
		}
		return false
	}
	return true
}

func containsWithIdentBoundary(haystack, needle string) bool {
	start := 0
	for {
		index := strings.Index(haystack[start:], needle)
		if index < 0 {
			return false
		}
		index += start
		beforeOK := index == 0 || !isIdentByte(haystack[index-1])
		after := index + len(needle)
		afterOK := after == len(haystack) || !isIdentByte(haystack[after])
		if beforeOK && afterOK {
			return true
		}
		start = index + 1
	}
}

func isIdentByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

var httpMethodSuffixes = []struct{ suffix, method string }{
	{".get", "GET"}, {".Get", "GET"}, {".GET", "GET"}, {".post", "POST"}, {".Post", "POST"},
	{".POST", "POST"}, {".put", "PUT"}, {".Put", "PUT"}, {".PUT", "PUT"},
	{".delete", "DELETE"}, {".Delete", "DELETE"}, {".DELETE", "DELETE"},
	{".patch", "PATCH"}, {".Patch", "PATCH"}, {".PATCH", "PATCH"},
	{".head", "HEAD"}, {".Head", "HEAD"}, {".HEAD", "HEAD"},
	{".options", "OPTIONS"}, {".Options", "OPTIONS"}, {"GetAsync", "GET"},
	{"PostAsync", "POST"}, {"PutAsync", "PUT"}, {"DeleteAsync", "DELETE"},
	{"SendAsync", ""}, {"getForObject", "GET"}, {"getForEntity", "GET"},
	{"postForObject", "POST"}, {"postForEntity", "POST"},
}

func serviceHTTPMethod(callee string) string {
	for _, candidate := range httpMethodSuffixes {
		if strings.HasSuffix(callee, candidate.suffix) {
			return candidate.method
		}
	}
	return ""
}

var routeMethodSuffixes = []struct{ suffix, method string }{
	{".GET", "GET"}, {".Get", "GET"}, {".get", "GET"}, {".POST", "POST"}, {".Post", "POST"}, {".post", "POST"},
	{".PUT", "PUT"}, {".Put", "PUT"}, {".put", "PUT"}, {".DELETE", "DELETE"}, {".Delete", "DELETE"}, {".delete", "DELETE"},
	{".PATCH", "PATCH"}, {".Patch", "PATCH"}, {".patch", "PATCH"}, {".Handle", "ANY"}, {".HandleFunc", "ANY"},
	{".handle", "ANY"}, {".Route", "ANY"}, {".route", "ANY"}, {"::get", "GET"}, {"::post", "POST"},
	{"::put", "PUT"}, {"::delete", "DELETE"}, {"::patch", "PATCH"}, {".MapGet", "GET"}, {".MapPost", "POST"},
	{".MapPut", "PUT"}, {".MapDelete", "DELETE"}, {".include_router", "ANY"}, {".mount", "ANY"},
	{".add_url_rule", "ANY"}, {".register_blueprint", "ANY"}, {".use", "ANY"}, {".register", "ANY"},
	{".add_route", "ANY"}, {".add_api_route", "ANY"}, {".add_api_websocket_route", "ANY"},
}

func serviceRouteMethod(callee string) string {
	for _, candidate := range routeMethodSuffixes {
		if strings.HasSuffix(callee, candidate.suffix) {
			return candidate.method
		}
	}
	return ""
}

func importedCallIdentity(language, callee string, imports []SyntaxFact) string {
	root := callee
	if dot := strings.IndexAny(root, ".:"); dot >= 0 {
		root = root[:dot]
	}
	for _, imported := range imports {
		importPath := strings.Trim(imported.Name, "\"'`")
		alias := imported.LocalName
		if alias == "" {
			alias = path.Base(strings.ReplaceAll(importPath, "\\", "/"))
		}
		if alias == root {
			return importPath + callee[len(root):]
		}
	}
	return callee
}
