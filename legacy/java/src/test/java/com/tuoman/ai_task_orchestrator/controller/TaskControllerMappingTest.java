package com.tuoman.ai_task_orchestrator.controller;

import org.junit.jupiter.api.Test;
import org.springframework.context.annotation.Profile;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PatchMapping;
import org.springframework.web.bind.annotation.RequestMapping;

import java.util.Arrays;

import static org.assertj.core.api.Assertions.assertThat;

class TaskControllerMappingTest {

    @Test
    void taskControllerShouldExposeAttemptsEndpoint() {
        boolean hasAttemptsEndpoint = Arrays.stream(TaskController.class.getDeclaredMethods())
                .map(method -> method.getAnnotation(GetMapping.class))
                .filter(annotation -> annotation != null)
                .flatMap(annotation -> Arrays.stream(annotation.value()))
                .anyMatch("/{taskId}/attempts"::equals);

        assertThat(hasAttemptsEndpoint).isTrue();
    }

    @Test
    void taskControllerShouldNotExposeStatusPatchEndpoint() {
        boolean hasStatusPatchEndpoint = Arrays.stream(TaskController.class.getDeclaredMethods())
                .map(method -> method.getAnnotation(PatchMapping.class))
                .filter(annotation -> annotation != null)
                .flatMap(annotation -> Arrays.stream(annotation.value()))
                .anyMatch("/{taskId}/status"::equals);

        assertThat(hasStatusPatchEndpoint).isFalse();
    }

    @Test
    void devTaskControllerShouldExposeStatusPatchOnlyInDevProfile() {
        RequestMapping requestMapping = DevTaskController.class.getAnnotation(RequestMapping.class);
        Profile profile = DevTaskController.class.getAnnotation(Profile.class);

        boolean hasDevStatusPatchEndpoint = Arrays.stream(DevTaskController.class.getDeclaredMethods())
                .map(method -> method.getAnnotation(PatchMapping.class))
                .filter(annotation -> annotation != null)
                .flatMap(annotation -> Arrays.stream(annotation.value()))
                .anyMatch("/{taskId}/status"::equals);

        assertThat(requestMapping.value()).containsExactly("/dev/tasks");
        assertThat(profile.value()).containsExactly("dev");
        assertThat(hasDevStatusPatchEndpoint).isTrue();
    }
}
