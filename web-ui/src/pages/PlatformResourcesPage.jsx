import FeaturePlaceholder from '../components/FeaturePlaceholder.jsx';

export default function PlatformResourcesPage() {
  return (
    <FeaturePlaceholder
      title="Platform Resources"
      description="Admin-only workspace for managing LLM / ASR / TTS resource catalogs."
      scope={[
        'Reserved for role=admin via route guard',
        'Supports service-level menu visibility by role',
        'Ready for CRUD integration with /api/v1/admin/platform-resources'
      ]}
      nextSteps={['table filters', 'create/edit form', 'resource version timeline']}
    />
  );
}
