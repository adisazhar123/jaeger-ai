CREATE CONSTRAINT service_name IF NOT EXISTS
FOR (s: Service) REQUIRE s.name IS UNIQUE

CREATE CONSTRAINT span_id IF NOT EXISTS
FOR (s: Span) REQUIRE s.span_id IS UNIQUE

CREATE VECTOR INDEX span_summary IF NOT EXISTS
FOR (s: Span)
ON s.embedding
OPTIONS { indexConfig: {
    `vector.dimensions`: 1536,
    `vector.similarity_function`: 'cosine'
}}
