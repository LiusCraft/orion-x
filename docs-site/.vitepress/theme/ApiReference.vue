<script setup>
import { onMounted } from "vue";

onMounted(() => {
	const reference = document.createElement("script");
	reference.id = "api-reference";
	reference.dataset.url = "/openapi/swagger.json";
	reference.dataset.configuration = JSON.stringify({
		layout: "modern",
		hideDownloadButton: false,
		hideModels: false,
	});
	document.body.appendChild(reference);

	const scalar = document.createElement("script");
	scalar.src = "https://cdn.jsdelivr.net/npm/@scalar/api-reference";
	document.body.appendChild(scalar);
});
</script>

<template>
	<div class="api-reference-root" />
</template>
