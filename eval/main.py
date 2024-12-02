from ragas import SingleTurnSample, evaluate
from ragas.metrics import ResponseRelevancy, Faithfulness, SemanticSimilarity, FactualCorrectness, RougeScore
import requests
from ragas.llms import LangchainLLMWrapper
from ragas.embeddings import LangchainEmbeddingsWrapper
from langchain_openai import ChatOpenAI
from langchain_openai import OpenAIEmbeddings
import argparse
import json


def inference(body):
    url = "http://localhost:54320/api/ask"
    headers = {
        "Content-Type": "application/json"
    }

    data = {
        "hop": body['hop'],
        "question": body['question'],
        "trace_id": body['trace_id'],
        "method": body['method']
    }

    try:
        response = requests.post(url, headers=headers, json=data, timeout=30)
        response.raise_for_status()
        return response.json()
    except requests.exceptions.Timeout:
        print("The request timed out after 30 seconds.")
    except requests.exceptions.RequestException as e:
        print(f"An error occurred: {e}")


def do_eval(hop, method):
    trace_id = 'e72ef241661424eb6970b65f6fd74b30'
    question_answers = [
        {
            'q': 'What distinct operation names are invoked by /dispatch?',
            'a': 'It invokes HTTP GET in the frontend service'
        },
        {
            'q': 'What is the customer ID?',
            'a': '731'
        },
        {
            'q': 'What distinct service names are affected by the Redis error?',
            'a': 'Redis-manual service and driver service'
        },
        {
            'q': 'What is the SQL operation performed by mysql?',
            'a': 'SQL SELECT'
        },
        {
            'q': 'What service invokes redis-manual service?',
            'a': 'Driver service'
        },
        {
            'q': 'Where is the location to find the driver?',
            'a': '94,287'
        },
        {
            'q': 'Why was there an error while finding a driver?',
            'a': 'This is a Redis timeout error'
        },
        {
            'q': 'Why did /dispatch API succeed despite a timeout in Redis?',
            'a': 'The call to Redis was retried multiple times until successful'
        },
        {
            'q': 'How many APIs do /dispatch invoke?',
            'a': '12 APIs',
        },
        {
            'q': 'How many errors occurred?',
            'a': '2 errors'
        },
        {
            'q': 'How many drivers were found?',
            'a': '10 drivers'
        },
        {
            'q': 'How many times did driver service invoke redis-manual?',
            'a': '13 times'
        },
        {
            'q': 'How many times to retry Redis?',
            'a': '2 times'
        },
        {
            'q': 'True or False. There are 2 instances of Redis errors.',
            'a': 'True'
        },
        {
            'q': 'True or False. There are 6 Redis errors.',
            'a': 'False'
        },
        {
            'q': 'True or False. The Redis error is caused by a timeout.',
            'a': 'True'
        },
        {
            'q': 'True or False. Driver ID T7991012 is found as a nearby driver.',
            'a': 'False'
        },
        {
            'q': 'True or False. Mysql service is called by customer service.',
            'a': 'True'
        },
        {
            'q': 'True or False. Mysql service is invoked by frontend.',
            'a': 'False'
        },
        {
            'q': 'True or False. Redis service calls driver service.',
            'a': 'False'
        },
        {
            'q': 'True or False. /route operation calls Redis service.',
            'a': 'False'
        },
        {
            'q': 'True or False. There are outgoing calls from Mysql service.',
            'a': 'False'
        },
        {
            'q': 'True or False. Frontend invokes customer service and route service.',
            'a': 'True'
        },
        {
            'q': 'True or False. Redis failed because of low disk space.',
            'a': 'False'
        },
        {
            'q': 'True or False. A write operation was performed by Mysql.',
            'a': 'False'
        },
        {
            'q': 'True or False. There is an indirect API call between frontend and redis.',
            'a': 'True'
        },
    ]

    evaluator_llm = LangchainLLMWrapper(ChatOpenAI(model="gpt-4o-mini-2024-07-18", temperature=0))
    evaluator_embeddings = LangchainEmbeddingsWrapper(OpenAIEmbeddings(model="text-embedding-3-small"))
    metrics = [
        # ResponseRelevancy(llm=evaluator_llm, embeddings=evaluator_embeddings),
        # Faithfulness(llm=evaluator_llm)
        # SemanticSimilarity(embeddings=evaluator_embeddings)
        # FactualCorrectness(llm=evaluator_llm)
        RougeScore()
    ]
    total_sim = 0
    count = 0
    logs = []
    for qa in question_answers:
        body = {"hop": hop, "question": qa['q'], "trace_id": trace_id, 'method': method}
        # print(f"sending inference for {body}")
        pred = inference(body)
        # print('pred', pred)
        sample = SingleTurnSample(
            response=pred['answer'],
            reference=qa['a']
            # user_input=qa.q,
            # response=pred['answer'],
            # retrieved_contexts=[
            #     pred['passage']
            # ]
        )
        msg = {
            'q': qa['q'],
            'r': pred['answer']
        }
        print(msg)
        logs.append(msg)
#         for metric in metrics:
#             score = metric.single_turn_score(sample)
#             if pred['answer'] == 'Insufficient Information':
#                 score = 0
#             print(f'metric: {metric.name}, score: {score}')
#             total_sim += score
#             count += 1
#         print('--------------------------------------------------')
    # print('avg', total_sim / count)
    # logs.append(f'avg: {total_sim / count}\n')
    with open(f"manual-scores-hop-{hop}-{method}.json", "a") as file:
        json.dump(logs, file, indent=4)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Send a POST request to an API.")
    parser.add_argument("--hop", type=int, required=True, help="The hop value for the request.")
    parser.add_argument("--method", type=str, required=True, help="The method value for the request.")

    args = parser.parse_args()

    do_eval(hop=args.hop, method=args.method)
