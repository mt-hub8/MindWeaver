package com.tuoman.ai_task_orchestrator.controller;

import org.springframework.stereotype.Controller;
import org.springframework.web.bind.annotation.GetMapping;

@Controller
public class ProductHomeController {

    @GetMapping("/")
    public String index() {
        return "forward:/index.html";
    }
}
