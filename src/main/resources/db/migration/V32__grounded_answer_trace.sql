ALTER TABLE rag_evaluation_case_result
    ADD COLUMN context_bundle_json MEDIUMTEXT NULL AFTER citations_json,
    ADD COLUMN citation_verification_json MEDIUMTEXT NULL AFTER context_bundle_json,
    ADD COLUMN unsupported_claim_report_json MEDIUMTEXT NULL AFTER citation_verification_json,
    ADD COLUMN refusal_decision_json MEDIUMTEXT NULL AFTER unsupported_claim_report_json,
    ADD COLUMN grounding_score_json MEDIUMTEXT NULL AFTER refusal_decision_json;
