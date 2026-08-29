package com.tuoman.ai_task_orchestrator.hybrid;

public interface LexicalRetriever {

    LexicalRetrievalResponse retrieve(LexicalRetrievalRequest request);

    String name();
}
