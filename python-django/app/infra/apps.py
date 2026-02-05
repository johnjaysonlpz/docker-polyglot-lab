import logging

from django.apps import AppConfig

log = logging.getLogger("infra")


class InfraConfig(AppConfig):
    default_auto_field = "django.db.models.BigAutoField"
    name = "infra"

    def ready(self) -> None:
        log.info("infra_app_ready", extra={"request_id": None})
