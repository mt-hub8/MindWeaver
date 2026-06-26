# AI-Task-Orchestrator

A Spring Boot based AI task orchestration system.

## Current Version

V0.1 - Create Task API

## Features in V0.1

- Create an AI task through `POST /tasks`
- Save task prompt into MySQL
- Initialize task status as `PENDING`
- Return `taskId` and `status`

## API

### Create Task

Request:

```http
POST /tasks
Content-Type: application/json

