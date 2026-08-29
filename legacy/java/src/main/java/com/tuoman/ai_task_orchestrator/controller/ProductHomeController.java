package com.tuoman.ai_task_orchestrator.controller;

import org.springframework.stereotype.Controller;
import org.springframework.web.bind.annotation.GetMapping;

@Controller
/**
 * V3.1 产品化首页入口。
 *
 * 只把根路径转发到静态产品页面，让用户从上传、文档管理和问答入口进入系统。
 * Controller 不承载业务策略，核心能力仍在后端 service/API 中。
 */
public class ProductHomeController {

    @GetMapping("/")
    public String index() {
        return "forward:/index.html";
    }
}
