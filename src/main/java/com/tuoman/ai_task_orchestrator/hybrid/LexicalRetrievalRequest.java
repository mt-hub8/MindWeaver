package com.tuoman.ai_task_orchestrator.hybrid;

public record LexicalRetrievalRequest(
        String query,
        int lexicalTopK,
        Long documentId
) {
}
