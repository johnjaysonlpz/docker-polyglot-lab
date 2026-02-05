package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import java.lang.reflect.Method;
import java.net.InetAddress;
import java.time.Duration;
import java.util.Set;
import org.apache.catalina.connector.Connector;
import org.junit.jupiter.api.Test;
import org.springframework.boot.tomcat.TomcatConnectorCustomizer;
import org.springframework.boot.tomcat.servlet.TomcatServletWebServerFactory;
import org.springframework.boot.web.server.WebServerFactoryCustomizer;

class ServerConfigurationTest {

  @Test
  void tomcatCustomizer_validHost_setsAddressPort_andRegistersConnectorCustomizer()
      throws Exception {
    ServiceProperties props = mock(ServiceProperties.class);

    when(props.getHost()).thenReturn("127.0.0.1");
    when(props.getPort()).thenReturn(18081);

    when(props.getTomcatConnectionTimeout()).thenReturn(Duration.ofMillis(1234));
    when(props.getTomcatKeepAliveTimeout()).thenReturn(Duration.ofMillis(5678));
    when(props.getTomcatMaxConnections()).thenReturn(111);
    when(props.getTomcatAcceptCount()).thenReturn(222);
    when(props.getTomcatMaxKeepAliveRequests()).thenReturn(333);
    when(props.getTomcatMaxThreads()).thenReturn(444);
    when(props.getTomcatMinSpareThreads()).thenReturn(55);

    ServerConfiguration cfg = new ServerConfiguration();
    WebServerFactoryCustomizer<TomcatServletWebServerFactory> customizer =
        cfg.tomcatCustomizer(props);

    TomcatServletWebServerFactory factory = new TomcatServletWebServerFactory();
    customizer.customize(factory);

    assertThat(factory.getAddress()).isEqualTo(InetAddress.getByName("127.0.0.1"));
    assertThat(factory.getPort()).isEqualTo(18081);

    Set<TomcatConnectorCustomizer> customizers = factory.getConnectorCustomizers();
    assertThat(customizers).hasSize(1);

    TomcatConnectorCustomizer cc = customizers.iterator().next();

    Connector connector = mock(Connector.class);
    cc.customize(connector);

    verify(connector).setProperty("connectionTimeout", "1234");
    verify(connector).setProperty("keepAliveTimeout", "5678");
    verify(connector).setProperty("maxConnections", "111");
    verify(connector).setProperty("acceptCount", "222");
    verify(connector).setProperty("maxKeepAliveRequests", "333");
    verify(connector).setProperty("maxThreads", "444");
    verify(connector).setProperty("minSpareThreads", "55");
  }

  @Test
  void tomcatCustomizer_unknownHost_doesNotSetAddress_butStillSetsPort_andRegistersCustomizer()
      throws Exception {
    ServiceProperties props = mock(ServiceProperties.class);

    when(props.getHost()).thenReturn("this-hostname-should-not-exist.invalid");
    when(props.getPort()).thenReturn(18082);

    when(props.getTomcatConnectionTimeout()).thenReturn(Duration.ofMillis(1));
    when(props.getTomcatKeepAliveTimeout()).thenReturn(Duration.ofMillis(2));
    when(props.getTomcatMaxConnections()).thenReturn(3);
    when(props.getTomcatAcceptCount()).thenReturn(4);
    when(props.getTomcatMaxKeepAliveRequests()).thenReturn(5);
    when(props.getTomcatMaxThreads()).thenReturn(6);
    when(props.getTomcatMinSpareThreads()).thenReturn(7);

    ServerConfiguration cfg = new ServerConfiguration();
    WebServerFactoryCustomizer<TomcatServletWebServerFactory> customizer =
        cfg.tomcatCustomizer(props);

    TomcatServletWebServerFactory factory = new TomcatServletWebServerFactory();
    customizer.customize(factory);

    assertThat(factory.getAddress()).isNull();
    assertThat(factory.getPort()).isEqualTo(18082);

    Set<TomcatConnectorCustomizer> customizers = factory.getConnectorCustomizers();
    assertThat(customizers).hasSize(1);

    TomcatConnectorCustomizer cc = customizers.iterator().next();

    Connector connector = mock(Connector.class);
    cc.customize(connector);

    verify(connector).setProperty("connectionTimeout", "1");
    verify(connector).setProperty("keepAliveTimeout", "2");
  }

  @Test
  void privateHelpers_safeMillis_and_resolveAddress_areCoveredViaReflection() throws Exception {
    ServerConfiguration cfg = new ServerConfiguration();

    Method safeMillis = ServerConfiguration.class.getDeclaredMethod("safeMillis", Duration.class);
    safeMillis.setAccessible(true);

    long ms1 = (long) safeMillis.invoke(cfg, Duration.ofMillis(123));
    assertThat(ms1).isEqualTo(123L);

    long big = (long) Integer.MAX_VALUE + 10_000L;
    long ms2 = (long) safeMillis.invoke(cfg, Duration.ofMillis(big));
    assertThat(ms2).isEqualTo((long) Integer.MAX_VALUE);

    Method resolveAddress =
        ServerConfiguration.class.getDeclaredMethod("resolveAddress", String.class);
    resolveAddress.setAccessible(true);

    Object ok = resolveAddress.invoke(cfg, "127.0.0.1");
    assertThat(ok).isInstanceOf(InetAddress.class);

    Object bad = resolveAddress.invoke(cfg, "this-hostname-should-not-exist.invalid");
    assertThat(bad).isNull();
  }
}
