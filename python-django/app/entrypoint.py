import os
import sys

def getenv(name: str, default: str) -> str:
    v = os.getenv(name, "")
    return v.strip() if v.strip() else default


def main() -> None:
    port = getenv("PORT", "8080")

    workers = getenv("GUNICORN_WORKERS", "2")
    worker_class = getenv("GUNICORN_WORKER_CLASS", "gthread")
    threads = getenv("GUNICORN_THREADS", "4")
    timeout = getenv("GUNICORN_TIMEOUT", "30")
    keepalive = getenv("GUNICORN_KEEPALIVE", "5")
    graceful_timeout = getenv("GUNICORN_GRACEFUL_TIMEOUT", "5")

    args = [
        "gunicorn",
        "django_app.wsgi:application",
        "--bind", f"0.0.0.0:{port}",
        "--workers", workers,
        "--worker-class", worker_class,
        "--threads", threads,
        "--timeout", timeout,
        "--keep-alive", keepalive,
        "--graceful-timeout", graceful_timeout,
    ]

    os.execvp(args[0], args)


if __name__ == "__main__":
    main()
