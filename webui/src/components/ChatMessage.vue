<script setup lang="ts">
import type { ChatRole } from "../types";

defineProps<{
  role: ChatRole;
  content: string;
}>();
</script>

<template>
  <div class="flex gap-2.5" :class="role === 'user' ? 'justify-end' : 'justify-start'">
    <div
      v-if="role === 'assistant'"
      class="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-vermilion-soft text-vermilion-dark"
      aria-hidden="true"
    >
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="1.8"
        class="h-4 w-4"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="m21 21-4.35-4.35M17 10.5a6.5 6.5 0 1 1-13 0 6.5 6.5 0 0 1 13 0Z"
        />
      </svg>
    </div>

    <div class="max-w-[85%] sm:max-w-[75%]">
      <p
        class="text-[11px] font-medium text-ink-muted"
        :class="role === 'user' ? 'text-right' : ''"
      >
        {{ role === "user" ? "Tu" : "Assistente" }}
      </p>
      <div
        class="mt-1 rounded-lg px-3.5 py-2.5 text-sm leading-6 whitespace-pre-line"
        :class="
          role === 'user'
            ? 'rounded-br-md bg-vermilion text-ink'
            : 'rounded-bl-md bg-paper text-ink'
        "
      >
        {{ content }}
      </div>
    </div>
  </div>
</template>
