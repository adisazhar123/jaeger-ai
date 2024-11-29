package clients

import (
	"context"
	"fmt"
	openai "github.com/sashabaranov/go-openai"
	"log"
	"os"
)

type OpenAIClient struct {
	client *openai.Client
}

func NewOpenAIClient() *OpenAIClient {
	token := os.Getenv("OPENAI_API_KEY")
	return &OpenAIClient{client: openai.NewClient(token)}
}

func (c *OpenAIClient) SummarizeSpan(ctx context.Context, passage string) (string, error) {
	prompt := `	
		You are to help a software engineer troubleshoot a distributed system. Summarize the given distributed tracing spans. Elaborate what you know about the span. 
		For example if it is a HTTP request explain briefly the flow of the request.
		Some examples are below. The raw span is delimited by <raw-span></raw-span>. The summary is delimited by <summary></summary>. 
		If there are extra keys, include them in your summary. Do not include the delimiter in your response.
		
		<raw-span>
		service name: member-service
		operation name: user-registration
		span id: 001
		duration: 100 nanoseconds
		start time: Nov 18, 2024
		span kind: client
		action kind: http
		</raw-span>
		<summary>
		The operation to create a new user in member service succeeded. It is associated with registering new customers when they sign up via the web application. It is a HTTP request that lasted 100 nano seconds. It had a span ID of 001.
		</summary>
`
	p := fmt.Sprintf(`
Here is the raw span you need to summarize.
<raw-span>
%s
</raw-span>
`, passage)
	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: prompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: p,
				},
			},
			Temperature: 0,
		},
	)

	if err != nil {
		log.Println("[SummarizeSpan] an error occurred", err)
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}

func (c *OpenAIClient) SummarizeLog(ctx context.Context, passage string) (string, error) {
	prompt := `
		You are to help a software engineer troubleshoot a distributed system. You are given logs that you need to summarize. If the log is empty return #EMPTY#. Some examples are below. The raw log is delimited by <raw-log></raw-log>. The summary is delimited by <summary></summary>. 
		If there are extra keys, include them in your summary. Do not include the delimiter in your response.
		
		<raw-log>
		Service is attempting to connect to Redis server
		Fetching key auth_token from Redis
		Found auth token, adding it to header
		API request to vendor is being made
		API response got 401 unauthorized
		</raw-log>
		<summary>
		An auth token was retrieved from Redis under key name auth_token. However, the API request using that auth token failed most likely due to the auth token being expired, indicated by the 401 status code.
		</summary>
		
		<raw-log>
		event: HTTP request received
		method: GET
		url: /customer?customer=123
		level: info
		</raw-log>
		
		<summary>
		A HTTP request at endpoint GET /customer was received with query parameter customer=123. This endpoint is used to retrieve a customer's personal details
		</summary>
		
		<raw-log>
		</raw-log>
		<summary>
		#EMPTY#
		</summary>
`
	p := fmt.Sprintf(`
		Here are the logs that you need to summarize
		<raw-log>
		%s
		</raw-log>
	`, passage)
	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: prompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: p,
				},
			},
			Temperature: 0,
		},
	)

	if err != nil {
		log.Println("[SummarizeLog] an error occurred", err)
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}

func (c *OpenAIClient) CreateEmbeddings(ctx context.Context, content string) ([]float32, error) {
	res, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Input: content,
		Model: openai.SmallEmbedding3,
	})
	if err != nil {
		log.Println("[SummarizeLog] an error occurred", err)
		return []float32{}, err
	}

	return res.Data[0].Embedding, nil
}

func (c *OpenAIClient) GenerateAnswer(ctx context.Context, query string, passage string) (string, error) {
	prompt := `
		You need to provide a factual answer based on the given question and passage. Use the passage to answer the question. 
		If there is not enough information in the passage, say "There is not enough information to give an answer." Be confident to say there is no answer.
		Here are some examples to show you.
		
		Example 1
		
		You are given a question and passage.
				
		Passage:
		
		This is a graph structure of a distributed tracing application.
		The nodes are spans. Each span has an ID and summary.
		The edges are of the format (span_id, relationship, span_id). This indicates that there is a directed relationship between spans.
		
		Edges:
		(01, INVOKES_CHILD, 02)
		(02, INVOKES_CHILD, 03)
		(01, INVOKES_CHILD, 04)
		
		Nodes:
		Span ID: 01
		Summary: User Joe initiated a money transfer to Bob.
		
		Span ID: 02
		Summary: Wallet service received request for a money transfer. It cannot do the transfer for unknown reasons.
		
		Span ID: 03
		Summary: Currency service cannot convert to the destination currency. It failed because exchange market is closed today.
		
		Span ID: 04
		Summary: Returning customer information. Joe is a gold tier member.
		
		Question: Why did the money transfer failed?
		Answer: An error occurred in currency service, span ID 03, because the exchange market is closed. This caused upstream services to failed.


		Example 2
		
		You are given a question and passage.
		
		Passage:
		
		This is a graph structure of a distributed tracing application.
		The nodes are spans. Each span has an ID and summary.
		The edges are of the format (span_id, relationship, span_id). This indicates that there is a directed relationship between spans.
		
		Edges:
		(01, INVOKES_CHILD, 02)
		(02, INVOKES_CHILD, 03)
		(01, INVOKES_CHILD, 04)
		
		Nodes:
		Span ID: 01
		Summary: User Joe initiated a money transfer to Bob.
		
		Span ID: 02
		Summary: Wallet service received request for a money transfer. It cannot do the transfer for unknown reasons.
		
		Span ID: 03
		Summary: Currency service cannot convert to the destination currency. It failed because exchange market is closed today.
		
		Span ID: 04
		Summary: Returning customer information. Joe is a gold tier member.

		Question: Why did the database connection shutdown?
		Answer:  There is not enough information to give an answer. The passage does not mention anything about database failures.
	`
	user := fmt.Sprintf(`
		Question: %s	
		Passage: %s
	`, query, passage)

	res, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4oMini20240718,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: prompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: user,
			},
		},
		Temperature: 0,
	})

	if err != nil {
		log.Println("[GenerateAnswer] an error occurred", err)
		return "", err
	}

	return res.Choices[0].Message.Content, nil
}
