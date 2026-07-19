<script setup>
import { onBeforeUnmount, onMounted, ref } from "vue";

const apiReferenceRoot = ref(null);
let apiReference;

onMounted(() => {
	const initialize = () => {
		apiReference = window.Scalar.createApiReference(apiReferenceRoot.value, {
			url: "/openapi/swagger.json",
			_integration: "html",
			layout: "modern",
			hideDownloadButton: false,
			hideModels: false,
		});
	};

	if (window.Scalar) {
		initialize();
		return;
	}

	const scalar = document.createElement("script");
	scalar.src = "https://cdn.jsdelivr.net/npm/@scalar/api-reference";
	scalar.onload = initialize;
	document.head.appendChild(scalar);
});

onBeforeUnmount(() => {
	apiReference?.destroy();
});
</script>

<template>
	<div ref="apiReferenceRoot" class="api-reference-root" />
</template>
