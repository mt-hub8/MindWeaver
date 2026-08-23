import json
import pathlib
import unittest


CONTRACT = pathlib.Path(__file__).with_name("openapi.json")
HTTP_METHODS = {"get", "post", "put", "patch", "delete", "options", "head", "trace"}


class OpenAPIContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.document = json.loads(CONTRACT.read_text(encoding="utf-8"))

    def operations(self):
        for path, path_item in self.document["paths"].items():
            for method, operation in path_item.items():
                if method in HTTP_METHODS:
                    yield path, method, operation

    def test_versioned_paths_and_unique_operation_ids(self):
        seen = set()
        self.assertEqual("3.1.0", self.document["openapi"])
        for path, _method, operation in self.operations():
            self.assertTrue(path.startswith("/api/v1/"), path)
            operation_id = operation.get("operationId")
            self.assertTrue(operation_id, (path, operation))
            self.assertNotIn(operation_id, seen)
            seen.add(operation_id)

    def test_mutations_declare_idempotency_and_csrf(self):
        for path, method, operation in self.operations():
            if method not in {"post", "put", "patch", "delete"}:
                continue
            refs = {item.get("$ref") for item in operation.get("parameters", [])}
            self.assertIn("#/components/parameters/IdempotencyKey", refs, (method, path))
            if path != "/api/v1/bootstrap/session":
                self.assertIn("#/components/parameters/CSRFToken", refs, (method, path))

    def test_problem_is_closed_and_has_required_codes(self):
        problem = self.document["components"]["schemas"]["Problem"]
        self.assertFalse(problem["additionalProperties"])
        codes = problem["properties"]["code"]["enum"]
        self.assertEqual(len(codes), len(set(codes)))
        self.assertTrue(
            {
                "EGRESS_DENIED",
                "CONTEXT_INSUFFICIENT",
                "CITATION_INVALID",
                "IDEMPOTENCY_KEY_REUSED",
                "PRECONDITION_FAILED",
            }.issubset(codes)
        )

    def test_all_error_bodies_are_problem_json(self):
        for path, method, operation in self.operations():
            for status, response in operation["responses"].items():
                if not str(status).startswith(("4", "5")):
                    continue
                if "$ref" in response:
                    self.assertTrue(response["$ref"].startswith("#/components/responses/"))
                    continue
                self.assertIn("application/problem+json", response.get("content", {}), (method, path, status))

    def test_sse_resume_and_buffering_contract(self):
        operation = self.document["paths"][
            "/api/v1/inference-invocations/{invocationId}/events"
        ]["get"]
        refs = {item.get("$ref") for item in operation["parameters"]}
        self.assertIn("#/components/parameters/LastEventID", refs)
        ok = operation["responses"]["200"]
        self.assertIn("X-Accel-Buffering", ok["headers"])
        self.assertIn("409", operation["responses"])


if __name__ == "__main__":
    unittest.main()
