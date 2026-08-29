package com.tuoman.ai_task_orchestrator.evaluation;

import com.tuoman.ai_task_orchestrator.vectorstore.VectorCountFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorDeleteFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorScanFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchRequest;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreOperationResult;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

/**
 * benchmark 专用 VectorStore 装饰器。
 *
 * 只统计 search latency，并把 upsert/delete/scan 原样委托给真实 VectorStore。
 * 它不能改变检索结果，否则 baseline/candidate 对比会失真。
 */
public class LatencyMeasuringVectorStore implements VectorStore {

    private final VectorStore delegate;

    private long totalSearchLatencyNanos;

    private int searchCount;

    private final List<Long> searchLatencyNanos = new ArrayList<>();

    public LatencyMeasuringVectorStore(VectorStore delegate) {
        this.delegate = delegate;
    }

    @Override
    public void upsert(List<VectorStoreDocument> documents) {
        delegate.upsert(documents);
    }

    @Override
    public List<VectorSearchResult> search(VectorSearchRequest request) {
        // latency 包含 Java 调用、序列化、HTTP/Docker 等本地环境成本。
        // 因此只适合相同环境下的相对比较，不应作为生产性能结论。
        long start = System.nanoTime();
        try {
            return delegate.search(request);
        } finally {
            long elapsed = System.nanoTime() - start;
            totalSearchLatencyNanos += elapsed;
            searchCount++;
            searchLatencyNanos.add(elapsed);
        }
    }

    @Override
    public void deleteByDocumentId(Long documentId) {
        delegate.deleteByDocumentId(documentId);
    }

    @Override
    public void deleteByDocumentIdAndProviderAndModel(Long documentId, String provider, String model) {
        delegate.deleteByDocumentIdAndProviderAndModel(documentId, provider, model);
    }

    @Override
    public VectorStoreOperationResult deleteByVectorId(String vectorId) {
        return delegate.deleteByVectorId(vectorId);
    }

    @Override
    public VectorStoreOperationResult deleteByStableVectorKey(String stableVectorKey) {
        return delegate.deleteByStableVectorKey(stableVectorKey);
    }

    @Override
    public VectorStoreOperationResult deleteByDocumentIdScoped(Long collectionId, Long documentId) {
        return delegate.deleteByDocumentIdScoped(collectionId, documentId);
    }

    @Override
    public VectorStoreOperationResult deleteByCollectionId(Long collectionId) {
        return delegate.deleteByCollectionId(collectionId);
    }

    @Override
    public VectorStoreOperationResult deleteByGeneration(Long collectionId, Long generation) {
        return delegate.deleteByGeneration(collectionId, generation);
    }

    @Override
    public VectorStoreOperationResult deleteByFilter(VectorDeleteFilter filter) {
        return delegate.deleteByFilter(filter);
    }

    @Override
    public long countByFilter(VectorCountFilter filter) {
        return delegate.countByFilter(filter);
    }

    @Override
    public List<VectorStoreDocument> scanByFilter(VectorScanFilter filter) {
        return delegate.scanByFilter(filter);
    }

    public long getTotalSearchLatencyNanos() {
        return totalSearchLatencyNanos;
    }

    public int getSearchCount() {
        return searchCount;
    }

    public LatencyStats toLatencyStats() {
        if (searchLatencyNanos.isEmpty()) {
            return LatencyStats.empty();
        }

        List<Long> sorted = new ArrayList<>(searchLatencyNanos);
        Collections.sort(sorted);
        long total = sorted.stream().mapToLong(Long::longValue).sum();
        int count = sorted.size();
        return new LatencyStats(
                count,
                total,
                millis(total / (double) count),
                millis(sorted.getFirst()),
                millis(sorted.getLast()),
                millis(percentile(sorted, 50)),
                millis(percentile(sorted, 95))
        );
    }

    private static double millis(double nanos) {
        return nanos / 1_000_000.0;
    }

    private static long percentile(List<Long> sorted, int percentile) {
        if (sorted.isEmpty()) {
            return 0L;
        }
        int index = (int) Math.ceil(percentile / 100.0 * sorted.size()) - 1;
        index = Math.max(0, Math.min(index, sorted.size() - 1));
        return sorted.get(index);
    }
}
