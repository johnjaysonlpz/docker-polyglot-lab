import os

os.environ.setdefault("DJANGO_SETTINGS_MODULE", "django_app.settings")

from django.core.asgi import get_asgi_application

from infra.logging_config import configure_logging_if_needed

configure_logging_if_needed()

application = get_asgi_application()
