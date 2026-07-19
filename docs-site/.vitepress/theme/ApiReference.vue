<script setup>
import { onBeforeUnmount, onMounted, ref } from "vue";

const apiReferenceRoot = ref(null);
const isLoading = ref(true);
const loadError = ref(false);
let apiReference;
let observer;
const apiSpecURL = import.meta.env.DEV
	? "/swagger/doc.json"
	: "/openapi/swagger.json";

const finishLoading = () => {
	if (!apiReferenceRoot.value?.querySelector(".scalar-api-reference")) {
		return;
	}

	isLoading.value = false;
	observer?.disconnect();
};

onMounted(() => {
	const initialize = () => {
		try {
			apiReference = window.Scalar.createApiReference(apiReferenceRoot.value, {
				url: apiSpecURL,
				_integration: "html",
				layout: "modern",
				hideDownloadButton: false,
				hideModels: false,
			});
			finishLoading();
		} catch {
			isLoading.value = false;
			loadError.value = true;
		}
	};

	observer = new MutationObserver(finishLoading);
	observer.observe(apiReferenceRoot.value, { childList: true, subtree: true });

	if (window.Scalar) {
		initialize();
		return;
	}

	const scalar = document.createElement("script");
	scalar.src = "https://cdn.jsdelivr.net/npm/@scalar/api-reference";
	scalar.onload = initialize;
	scalar.onerror = () => {
		isLoading.value = false;
		loadError.value = true;
	};
	document.head.appendChild(scalar);
});

onBeforeUnmount(() => {
	observer?.disconnect();
	apiReference?.destroy();
});
</script>

<template>
	<div class="api-reference-shell">
		<div ref="apiReferenceRoot" class="api-reference-root" />
		<div v-if="isLoading" class="api-reference-loading" role="status" aria-live="polite">
			<span class="api-reference-spinner" aria-hidden="true" />
			<span>正在加载 API 文档</span>
		</div>
		<div v-else-if="loadError" class="api-reference-error" role="alert">
			无法加载 API 文档，请稍后刷新重试。
		</div>
	</div>
</template>
