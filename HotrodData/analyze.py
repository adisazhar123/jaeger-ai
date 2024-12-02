import os
import openai
import json

class OpenAITraceAnalyzer:
    def __init__(self, model="gpt-3.5-turbo"):
        # Retrieve the API key from environment variables
        self.api_key = os.getenv("OPENAI_API_KEY")
        if not self.api_key:
            raise EnvironmentError("OpenAI API key not found in environment variables.")
        openai.api_key = self.api_key
        self.model = model

    def generate_prompt(self, task, json_data):
        """
        Generates a concise prompt based on the task and filtered JSON data.
        """
        if task == "list_errors":
            filtered_data = [
                {"spanID": span.get("spanID"), "warnings": span.get("warnings"), "logs": span.get("logs")}
                for trace in json_data.get("data", [])
                for span in trace.get("spans", [])
            ]
            return f"Analyze the following JSON trace data and list all errors:\n{json.dumps(filtered_data, indent=2)}"
        elif task == "summarize_trace":
            trace_id = json_data.get("traceID", "unknown")
            return f"Summarize the trace with Trace ID '{trace_id}'. Provide an overview of spans and errors:\n{json.dumps(json_data, indent=2)}"
        elif task == "find_http_errors":
            filtered_data = [
                {"spanID": span.get("spanID"), "tags": [tag for tag in span.get("tags", []) if tag.get("key") == "http.status_code"]}
                for trace in json_data.get("data", [])
                for span in trace.get("spans", [])
            ]
            return f"Find spans with HTTP status codes other than 200:\n{json.dumps(filtered_data, indent=2)}"
        else:
            return "Unsupported task."

    def query_openai(self, prompt):
        """
        Queries OpenAI's API with the generated prompt.
        """
        response = openai.ChatCompletion.create(
            model=self.model,
            messages=[
                {"role": "system", "content": "You are a JSON trace data analyst."},
                {"role": "user", "content": prompt}
            ],
            temperature=0.7,
            max_tokens=500
        )
        return response['choices'][0]['message']['content'].strip()

    def analyze_large_json(self, task, json_data):
        """
        Analyzes large JSON data by processing it in smaller chunks.
        """
        results = []
        for trace in json_data.get("data", []):
            prompt = self.generate_prompt(task, {"data": [trace]})
            result = self.query_openai(prompt)
            results.append(result)
        return "\n".join(results)


# Main Program
if __name__ == "__main__":
    analyzer = OpenAITraceAnalyzer()

    for i in range(1, 20):  # Iterate through files hotrod1.json to hotrod10.json
        file_path = f"./SampleData/hotrod{i}.json"
        print(f"\nProcessing file: {file_path}")

        try:
            # Load JSON trace data from file
            with open(file_path, "r") as f:
                json_data = json.load(f)
        except FileNotFoundError:
            print(f"Error: File '{file_path}' not found. Skipping.")
            continue
        except json.JSONDecodeError:
            print(f"Error: Failed to parse JSON file '{file_path}'. Skipping.")
            continue

        # Task 1: List Errors
        print("\nTask 1: List Errors")
        try:
            errors = analyzer.analyze_large_json("list_errors", json_data)
            print(errors)
        except Exception as e:
            print(f"Error during analysis: {e}")

        # Task 2: Summarize a Trace
        print("\nTask 2: Summarize a Trace")
        try:
            summary = analyzer.analyze_large_json("summarize_trace", json_data)
            print(summary)
        except Exception as e:
            print(f"Error during analysis: {e}")

        # Task 3: Find HTTP Errors
        print("\nTask 3: Find HTTP Errors")
        try:
            http_errors = analyzer.analyze_large_json("find_http_errors", json_data)
            print(http_errors)
        except Exception as e:
            print(f"Error during analysis: {e}")
