package com.tuoman.ai_task_orchestrator.kbhealth;

public final class RagHealthDisplayTexts {

    private RagHealthDisplayTexts() {
    }

    public static String datasetType(RagEvaluationDatasetType type) {
        if (type == null) {
            return "未知";
        }
        return switch (type) {
            case GOLD_TEST_SET -> "自建 Gold Test Set";
            case T2RANKING_SUBSET -> "T2Ranking 子集";
            case DUREADER_RETRIEVAL_SUBSET -> "DuReader-Retrieval 子集";
            case CRUD_RAG_SUBSET -> "CRUD-RAG 子集";
            case CUSTOM -> "自定义评测集";
        };
    }

    public static String strategy(RagEvaluationRetrievalStrategy strategy) {
        if (strategy == null) {
            return "未知";
        }
        return switch (strategy) {
            case VECTOR_ONLY -> "纯向量检索";
            case BM25_ONLY -> "BM25 关键词检索";
            case VECTOR_WITH_METADATA_FILTER -> "向量 + 元数据过滤";
            case HYBRID -> "混合检索";
            case HYBRID_RRF -> "混合检索 + RRF 融合";
            case HYBRID_RRF_RERANK -> "混合检索 + RRF + 重排序";
            case HYBRID_RRF_RERANK_PARENT_CONTEXT -> "混合检索 + RRF + 重排序 + 父级上下文回填";
        };
    }

    public static String scoringProfile(RagHealthScoringProfile profile) {
        if (profile == null) {
            return "未知";
        }
        return switch (profile) {
            case BALANCED -> "平衡模式";
            case PRECISE -> "精准模式";
            case COMPREHENSIVE -> "全面模式";
            case GENERATION_TRUST -> "生成可信模式";
        };
    }

    public static String scoreLevel(int score) {
        if (score < 0) {
            return "暂无评分";
        }
        if (score >= 90) {
            return "优秀";
        }
        if (score >= 75) {
            return "良好";
        }
        if (score >= 60) {
            return "一般";
        }
        return "较差";
    }
}
