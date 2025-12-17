from django.apps import AppConfig
import logging

log = logging.getLogger("infra")

class InfraConfig(AppConfig):
    default_auto_field = "django.db.models.BigAutoField"
    name = "infra"

    def ready(self):
        log.info("infra_app_ready")
