package com.tuoman.ai_task_orchestrator.service.kbhealth;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.CreateRagEvaluationDatasetRequest;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.ImportRagEvaluationCasesRequest;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.ImportRagEvaluationCasesResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.RagEvaluationCaseResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.RagEvaluationDatasetResponse;
import com.tuoman.ai_task_orchestrator.dto.kbhealth.UpdateRagEvaluationCaseRequest;
import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseEntity;
import com.tuoman.ai_task_orchestrator.entity.RagEvaluationDatasetEntity;
import com.tuoman.ai_task_orchestrator.kbhealth.JsonFieldCodec;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationDatasetStatus;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationDatasetType;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationQueryType;
import com.tuoman.ai_task_orchestrator.kbhealth.RagHealthDisplayTexts;
import com.tuoman.ai_task_orchestrator.repository.RagEvaluationCaseRepository;
import com.tuoman.ai_task_orchestrator.repository.RagEvaluationDatasetRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.ArrayList;
import java.util.Iterator;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.UUID;

/**
 * V14 RAG Evaluation Dataset 服务。
 *
 * Dataset 是一组评测 Case 的容器；Case 保存 query、expectedDocIds、expectedChunkIds、
 * negativeDocIds、metadataFilter 和 expectedAnswerPoints 等 gold labels。
 *
 * Gold Test Set 重要在于它让检索/生成优化可以被重复比较，而不是只凭单次体感判断。
 */
@Service
@RequiredArgsConstructor
public class RagEvaluationDatasetService {

    private final RagEvaluationDatasetRepository datasetRepository;

    private final RagEvaluationCaseRepository caseRepository;

    private final ObjectMapper objectMapper;

    @Transactional
    public RagEvaluationDatasetResponse createDataset(CreateRagEvaluationDatasetRequest request) {
        if (request == null || request.getName() == null || request.getName().isBlank()) {
            throw BusinessException.validationError("name is required");
        }
        RagEvaluationDatasetEntity entity = new RagEvaluationDatasetEntity();
        entity.setName(request.getName().trim());
        entity.setDescription(request.getDescription());
        entity.setDatasetType(request.getDatasetType() == null ? RagEvaluationDatasetType.GOLD_TEST_SET : request.getDatasetType());
        entity.setStatus(request.getStatus() == null ? RagEvaluationDatasetStatus.DRAFT : request.getStatus());
        entity.setSourceType(request.getSourceType());
        entity.setCaseCount(0);
        RagEvaluationDatasetEntity saved = datasetRepository.save(entity);
        return toDatasetResponse(saved);
    }

    @Transactional(readOnly = true)
    public List<RagEvaluationDatasetResponse> listDatasets() {
        return datasetRepository.findAll().stream().map(this::toDatasetResponse).toList();
    }

    @Transactional(readOnly = true)
    public RagEvaluationDatasetResponse getDataset(Long datasetId) {
        return toDatasetResponse(findDatasetOrThrow(datasetId));
    }

    @Transactional(readOnly = true)
    public List<RagEvaluationCaseResponse> listCases(Long datasetId) {
        findDatasetOrThrow(datasetId);
        return caseRepository.findByDatasetIdOrderByIdAsc(datasetId).stream()
                .map(this::toCaseResponse)
                .toList();
    }

    @Transactional
    public ImportRagEvaluationCasesResponse importCases(Long datasetId, ImportRagEvaluationCasesRequest request) {
        // 导入 case 时保留 expected ids、negative ids 和 metadata filter。
        // 这些字段决定后续 Recall@K、WrongVersionLeakRate、CitationAccuracy 等指标是否可计算。
        RagEvaluationDatasetEntity dataset = findDatasetOrThrow(datasetId);
        if (request == null || request.getPayload() == null || request.getPayload().isBlank()) {
            throw BusinessException.validationError("payload is required");
        }
        String format = request.getFormat() == null ? "json" : request.getFormat().trim().toLowerCase(Locale.ROOT);
        List<Map<String, Object>> rawCases = "csv".equals(format)
                ? parseCsvCases(request.getPayload())
                : parseJsonCases(request.getPayload());

        int imported = 0;
        int skipped = 0;
        int failed = 0;
        List<String> errors = new ArrayList<>();

        for (int i = 0; i < rawCases.size(); i++) {
            Map<String, Object> raw = rawCases.get(i);
            try {
                String query = stringValue(raw.get("query"));
                if (query == null || query.isBlank()) {
                    failed++;
                    errors.add("row " + (i + 1) + ": query is required");
                    continue;
                }
                String caseId = stringValue(raw.get("case_id"));
                if (caseId == null || caseId.isBlank()) {
                    caseId = "rag_case_" + UUID.randomUUID().toString().substring(0, 8);
                }
                if (caseRepository.findByDatasetIdAndCaseId(datasetId, caseId).isPresent()) {
                    skipped++;
                    continue;
                }
                RagEvaluationCaseEntity entity = new RagEvaluationCaseEntity();
                entity.setDatasetId(datasetId);
                entity.setCaseId(caseId);
                entity.setQuery(query);
                entity.setQueryType(parseQueryType(stringValue(raw.get("query_type"))));
                entity.setCollectionId(parseLong(raw.get("collection_id")));
                entity.setExpectedDocIdsJson(JsonFieldCodec.write(raw.get("expected_doc_ids")));
                entity.setExpectedChunkIdsJson(JsonFieldCodec.write(raw.get("expected_chunk_ids")));
                entity.setExpectedRankJson(JsonFieldCodec.write(raw.get("expected_rank")));
                entity.setNegativeDocIdsJson(JsonFieldCodec.write(raw.get("negative_doc_ids")));
                entity.setExpectedAnswerPointsJson(JsonFieldCodec.write(raw.get("expected_answer_points")));
                entity.setAnswerMustCite(parseBoolean(raw.get("answer_must_cite")));
                entity.setMetadataFilterJson(JsonFieldCodec.write(raw.get("metadata_filter")));
                entity.setDifficulty(stringValue(raw.get("difficulty")));
                entity.setTagsJson(JsonFieldCodec.write(raw.get("tags")));
                entity.setEnabled(true);
                caseRepository.save(entity);
                imported++;
            } catch (Exception exception) {
                failed++;
                errors.add("row " + (i + 1) + ": " + exception.getMessage());
            }
        }

        refreshCaseCount(dataset);
        return new ImportRagEvaluationCasesResponse(rawCases.size(), imported, skipped, failed, errors);
    }

    @Transactional
    public RagEvaluationCaseResponse updateCase(Long caseId, UpdateRagEvaluationCaseRequest request) {
        RagEvaluationCaseEntity entity = caseRepository.findById(caseId)
                .orElseThrow(() -> BusinessException.invalidRequest("case not found: " + caseId));
        if (request.getQuery() != null) {
            entity.setQuery(request.getQuery());
        }
        if (request.getQueryType() != null) {
            entity.setQueryType(request.getQueryType());
        }
        if (request.getCollectionId() != null) {
            entity.setCollectionId(request.getCollectionId());
        }
        if (request.getExpectedDocIds() != null) {
            entity.setExpectedDocIdsJson(JsonFieldCodec.write(request.getExpectedDocIds()));
        }
        if (request.getExpectedChunkIds() != null) {
            entity.setExpectedChunkIdsJson(JsonFieldCodec.write(request.getExpectedChunkIds()));
        }
        if (request.getExpectedRank() != null) {
            entity.setExpectedRankJson(JsonFieldCodec.write(request.getExpectedRank()));
        }
        if (request.getNegativeDocIds() != null) {
            entity.setNegativeDocIdsJson(JsonFieldCodec.write(request.getNegativeDocIds()));
        }
        if (request.getExpectedAnswerPoints() != null) {
            entity.setExpectedAnswerPointsJson(JsonFieldCodec.write(request.getExpectedAnswerPoints()));
        }
        if (request.getAnswerMustCite() != null) {
            entity.setAnswerMustCite(request.getAnswerMustCite());
        }
        if (request.getMetadataFilter() != null) {
            entity.setMetadataFilterJson(JsonFieldCodec.write(request.getMetadataFilter()));
        }
        if (request.getDifficulty() != null) {
            entity.setDifficulty(request.getDifficulty());
        }
        if (request.getTags() != null) {
            entity.setTagsJson(JsonFieldCodec.write(request.getTags()));
        }
        if (request.getEnabled() != null) {
            entity.setEnabled(request.getEnabled());
        }
        RagEvaluationCaseEntity saved = caseRepository.save(entity);
        refreshCaseCount(datasetRepository.findById(entity.getDatasetId()).orElseThrow());
        return toCaseResponse(saved);
    }

    @Transactional
    public void deleteCase(Long caseId) {
        RagEvaluationCaseEntity entity = caseRepository.findById(caseId)
                .orElseThrow(() -> BusinessException.invalidRequest("case not found: " + caseId));
        entity.setEnabled(false);
        caseRepository.save(entity);
        datasetRepository.findById(entity.getDatasetId()).ifPresent(this::refreshCaseCount);
    }

    private void refreshCaseCount(RagEvaluationDatasetEntity dataset) {
        dataset.setCaseCount((int) caseRepository.countByDatasetIdAndEnabledTrue(dataset.getId()));
        datasetRepository.save(dataset);
    }

    private RagEvaluationDatasetEntity findDatasetOrThrow(Long datasetId) {
        return datasetRepository.findById(datasetId)
                .orElseThrow(() -> BusinessException.invalidRequest("dataset not found: " + datasetId));
    }

    private List<Map<String, Object>> parseJsonCases(String payload) {
        try {
            JsonNode root = objectMapper.readTree(payload);
            if (root.isArray()) {
                return objectMapper.convertValue(root, new TypeReference<List<Map<String, Object>>>() {
                });
            }
            JsonNode cases = root.get("cases");
            if (cases != null && cases.isArray()) {
                return objectMapper.convertValue(cases, new TypeReference<List<Map<String, Object>>>() {
                });
            }
            throw BusinessException.validationError("JSON payload must be an array or contain cases[]");
        } catch (BusinessException exception) {
            throw exception;
        } catch (Exception exception) {
            throw BusinessException.validationError("invalid JSON payload: " + exception.getMessage());
        }
    }

    private List<Map<String, Object>> parseCsvCases(String payload) {
        String[] lines = payload.split("\\R");
        if (lines.length < 2) {
            return List.of();
        }
        String[] headers = lines[0].split(",", -1);
        List<Map<String, Object>> cases = new ArrayList<>();
        for (int i = 1; i < lines.length; i++) {
            if (lines[i].isBlank()) {
                continue;
            }
            String[] values = lines[i].split(",", -1);
            Map<String, Object> row = new java.util.LinkedHashMap<>();
            for (int h = 0; h < headers.length && h < values.length; h++) {
                String header = headers[h].trim();
                String value = values[h].trim();
                if ("expected_doc_ids".equals(header) || "expected_chunk_ids".equals(header)
                        || "negative_doc_ids".equals(header) || "expected_answer_points".equals(header)
                        || "metadata_filter".equals(header) || "tags".equals(header) || "expected_rank".equals(header)) {
                    row.put(header, parseJsonCell(value));
                } else if ("collection_id".equals(header)) {
                    row.put(header, parseLong(value));
                } else if ("answer_must_cite".equals(header)) {
                    row.put(header, parseBoolean(value));
                } else {
                    row.put(header, value);
                }
            }
            cases.add(row);
        }
        return cases;
    }

    private RagEvaluationDatasetResponse toDatasetResponse(RagEvaluationDatasetEntity entity) {
        return new RagEvaluationDatasetResponse(
                entity.getId(),
                entity.getName(),
                entity.getDescription(),
                entity.getDatasetType(),
                RagHealthDisplayTexts.datasetType(entity.getDatasetType()),
                entity.getStatus(),
                entity.getCaseCount(),
                entity.getSourceType(),
                entity.getCreatedAt(),
                entity.getUpdatedAt()
        );
    }

    private RagEvaluationCaseResponse toCaseResponse(RagEvaluationCaseEntity entity) {
        return new RagEvaluationCaseResponse(
                entity.getId(),
                entity.getDatasetId(),
                entity.getCaseId(),
                entity.getQuery(),
                entity.getQueryType(),
                entity.getCollectionId(),
                JsonFieldCodec.readLongList(entity.getExpectedDocIdsJson()),
                JsonFieldCodec.readLongList(entity.getExpectedChunkIdsJson()),
                JsonFieldCodec.readStringList(entity.getExpectedRankJson()),
                JsonFieldCodec.readLongList(entity.getNegativeDocIdsJson()),
                JsonFieldCodec.readStringList(entity.getExpectedAnswerPointsJson()),
                entity.getAnswerMustCite(),
                JsonFieldCodec.readMap(entity.getMetadataFilterJson()),
                entity.getDifficulty(),
                JsonFieldCodec.readStringList(entity.getTagsJson()),
                entity.getEnabled(),
                entity.getCreatedAt(),
                entity.getUpdatedAt()
        );
    }

    private RagEvaluationQueryType parseQueryType(String raw) {
        if (raw == null || raw.isBlank()) {
            return null;
        }
        String normalized = raw.trim().toUpperCase(Locale.ROOT);
        try {
            return RagEvaluationQueryType.valueOf(normalized);
        } catch (IllegalArgumentException exception) {
            for (RagEvaluationQueryType type : RagEvaluationQueryType.values()) {
                if (type.name().equalsIgnoreCase(normalized) || type.name().replace('_', ' ').equalsIgnoreCase(raw)) {
                    return type;
                }
            }
            return null;
        }
    }

    private String stringValue(Object value) {
        return value == null ? null : String.valueOf(value);
    }

    private Long parseLong(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof Number number) {
            return number.longValue();
        }
        try {
            return Long.parseLong(String.valueOf(value));
        } catch (NumberFormatException exception) {
            return null;
        }
    }

    private Object parseJsonCell(String value) {
        if (value == null || value.isBlank()) {
            return null;
        }
        try {
            return objectMapper.readValue(value, Object.class);
        } catch (Exception exception) {
            return value;
        }
    }

    private Boolean parseBoolean(Object value) {
        if (value == null) {
            return false;
        }
        if (value instanceof Boolean bool) {
            return bool;
        }
        return Boolean.parseBoolean(String.valueOf(value));
    }
}
