package com.tuoman.ai_task_orchestrator.hybrid;

public interface FusionRanker {

    FusionResponse fuse(FusionRequest request, int rrfK);

    String strategy();
}
