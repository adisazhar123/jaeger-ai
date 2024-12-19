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
		The operation "user-registration" to create a new user in member-service succeeded. It is associated with registering new customers when they sign up via the web application. It is a HTTP request that lasted 100 nano seconds. Its span ID is 001.
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

func (c *OpenAIClient) GenerateAnswer(ctx context.Context, query string, passage string, method string) (string, error) {
	prompt := `
		You need to provide a factual answer based on the given question and passage. Use the passage to answer the question.
		If you believe the question cannot be answered from the given passage return the phrase "Insufficient Information". Keep the answer concise and specific. Do not include redundant information.

		Here are some examples to show you. The passage is delimited by <passage></passage>, question is delimited by <question></question>, and answer is delimited by <answer></answer>. You are also given <explanation></explanation> to help you reason how to arrive at an answer. Do not include <explanation></explanation> in your response.

		The passage is a graph structure of a distributed tracing application.
		The nodes are spans. Each span has an ID and summary.
		The edges are of the format (span_id, relationship, span_id). This indicates that there is a directed relationship between spans.
		It is important that you use the (span_id, relationship, span_id) format to help you reason about the answer. 
		Do not include the edge format in your answer unless asked to.
				
		<passage>	
		Edge types:
		INVOKES_CHILD means that a span calls or invokes another span 
		
		Edges:
		(01, INVOKES_CHILD, 02)
		(02, INVOKES_CHILD, 03)
		(01, INVOKES_CHILD, 04)
		
		Nodes:
		Span ID: 01
		Operation: initiate-transfer
		Summary: User Joe initiated a money transfer to Bob.
		
		Span ID: 02
		Operation: wallet-processor
		Summary: Wallet service received request for a money transfer. It cannot do the transfer for unknown reasons.
		
		Span ID: 03
		Operation: convert-currency
		Summary: Currency service cannot convert to the destination currency. It failed because exchange market is closed today.
		
		Span ID: 04
		Operation: get-customer
		Summary: Returning customer information. Joe is a gold tier member.
		</passage>
		
		<question>
		Why did the money transfer failed?
		</question>
		<explanation>
		The passage says that "It failed because exchange market is closed today.". 
		</<explanation>
		<answer>
		An error occurred in currency service because the exchange market is closed.
		</answer>	

		<question> 
		Why did the database connection shutdown?
		</question>
		<explanation>
		Database connection shutdown is not mentioned anywhere in the passage.
		</explanation>
		<answer>
		Insufficient Information
		</answer>

		<question> 
		Which span did the market closure occurred in?
		</question>
		<explanation>
		The passage mentions "Span ID: 03" where "It failed because exchange market is closed today.".
		</explanation>
		<answer>
		Span ID 03
		</answer>

		<question>
		What is the operation name invoked by operation initiate-transfer? 
		</question>
		<explanation>
		operation "initiate-transfer" is in Span ID: 01 which invokes Span ID: 02 which is wallet-processor and Span ID: 04 which is get-customer.
		</explanation>
		<answer>
		wallet-processor and get-customer.
		</answer>
	`

	if method == "naive-rag" {
		prompt = `
		You need to provide a factual answer based on the given question and passage. Use the passage to answer the question.
		If you believe the question cannot be answered from the given passage return the phrase "Insufficient Information". Keep the answer concise and specific. Do not include redundant information.
	`
	}

	user := fmt.Sprintf(`
		Keep the answer short, brief, and specific. If asked for a count return the number. Do not include redundant information.

		<question> 
		%s 
		</question>	
		<passage> 
		%s 
		</passage>
	`, query, passage)

	res, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4oMini20240718,
		//Model: openai.GPT4Turbo,
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
