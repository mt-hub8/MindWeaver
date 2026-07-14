package com.tuoman.ai_task_orchestrator;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.scheduling.annotation.EnableScheduling;

@SpringBootApplication
@EnableScheduling
/**
 * 应用启动入口。
 *
 * V0.4.1 之后数据库结构由 Flyway migration 管理，运行时只做 schema validate，
 * 不应依赖 Hibernate 自动改表或在启动代码里修复 schema。
 *
 * V0.11 的本地 infra 约束也从这里生效：MySQL、RabbitMQ、profile/env 配置由外部环境提供，
 * 代码不能写死本地账号、端口或密钥。
 */
public class AiTaskOrchestratorApplication {

	public static void main(String[] args) {

		SpringApplication.run(AiTaskOrchestratorApplication.class, args);
	}

}
