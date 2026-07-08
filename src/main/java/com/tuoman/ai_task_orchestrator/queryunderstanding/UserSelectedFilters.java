package com.tuoman.ai_task_orchestrator.queryunderstanding;

import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentDocType;
import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
public class UserSelectedFilters {

    private final Long collectionId;

    private final DocumentDocType docType;

    private final String version;

    private final String source;

    private final ChunkMetadataStatus status;

    private final List<String> tags;

    public static UserSelectedFilters ofCollection(Long collectionId) {
        return UserSelectedFilters.builder().collectionId(collectionId).build();
    }

    public boolean hasCollection() {
        return collectionId != null;
    }
}
