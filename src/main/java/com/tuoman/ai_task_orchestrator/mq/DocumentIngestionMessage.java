package com.tuoman.ai_task_orchestrator.mq;

import lombok.AllArgsConstructor;
import lombok.Getter;
import lombok.NoArgsConstructor;

@Getter
@NoArgsConstructor
@AllArgsConstructor
public class DocumentIngestionMessage {

    private Long taskId;

    private Long documentId;
}
