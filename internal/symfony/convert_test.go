package symfony

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func convertString(t *testing.T, content string) string {
	t.Helper()

	container, err := ParseServicesXML([]byte(content))
	require.NoError(t, err)

	converted, err := ConvertContainerToYAML(container)
	require.NoError(t, err)

	// Everything we generate has to be parseable YAML.
	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal(converted, &parsed))

	return string(converted)
}

func convertErr(t *testing.T, content string) error {
	t.Helper()

	container, err := ParseServicesXML([]byte(content))
	require.NoError(t, err)

	_, err = ConvertContainerToYAML(container)
	require.Error(t, err)

	return err
}

func TestConvertBasicService(t *testing.T) {
	output := convertString(t, `<?xml version="1.0" ?>
<container xmlns="http://symfony.com/schema/dic/services"
           xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
           xsi:schemaLocation="http://symfony.com/schema/dic/services http://symfony.com/schema/dic/services/services-1.0.xsd">
    <services>
        <service id="Shop\Service\OrderService">
            <argument type="service" id="logger"/>
            <argument>%shop.order_limit%</argument>
            <argument>true</argument>
            <argument>42</argument>
            <tag name="kernel.event_subscriber"/>
        </service>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\Service\OrderService:
        arguments:
            - '@logger'
            - '%shop.order_limit%'
            - true
            - 42
        tags:
            - kernel.event_subscriber
`, output)
}

func TestConvertServiceWithDifferentClassAndId(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="shop.order_service" class="Shop\Service\OrderService" public="true"/>
        <service id="Shop\Service\SameClass" class="Shop\Service\SameClass"/>
    </services>
</container>`)

	assert.Equal(t, `services:
    shop.order_service:
        class: Shop\Service\OrderService
        public: true

    Shop\Service\SameClass: ~
`, output)
}

func TestConvertServiceAttributes(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\A" class="Shop\A" public="false" shared="false" synthetic="true" lazy="true"
                 abstract="true" autowire="true" autoconfigure="false" constructor="create"/>
        <service id="Shop\B" class="Shop\B" parent="Shop\A" lazy="Shop\SomeInterface"/>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\A:
        lazy: true
        shared: false
        synthetic: true
        abstract: true
        public: false
        autowire: true
        autoconfigure: false
        constructor: create

    Shop\B:
        parent: Shop\A
        lazy: Shop\SomeInterface
`, output)
}

func TestConvertDecoration(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\Decorator" class="Shop\Decorator" decorates="Shop\Original"
                 decoration-inner-name="Shop\Original.inner" decoration-priority="5" decoration-on-invalid="ignore"/>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\Decorator:
        decorates: Shop\Original
        decoration_inner_name: Shop\Original.inner
        decoration_priority: 5
        decoration_on_invalid: ignore
`, output)
}

func TestConvertParameters(t *testing.T) {
	output := convertString(t, `<container>
    <parameters>
        <parameter key="shop.string">hello</parameter>
        <parameter key="shop.bool">true</parameter>
        <parameter key="shop.int">42</parameter>
        <parameter key="shop.float">1.5</parameter>
        <parameter key="shop.null">null</parameter>
        <parameter key="shop.forced_string" type="string">true</parameter>
        <parameter key="shop.constant" type="constant">Shop\Config::LIMIT</parameter>
        <parameter key="shop.list" type="collection">
            <parameter>one</parameter>
            <parameter>two</parameter>
        </parameter>
        <parameter key="shop.map" type="collection">
            <parameter key="a">1</parameter>
            <parameter key="b" type="collection">
                <parameter>nested</parameter>
            </parameter>
        </parameter>
    </parameters>
</container>`)

	assert.Equal(t, `parameters:
    shop.string: hello
    shop.bool: true
    shop.int: 42
    shop.float: 1.5
    shop.null: null
    shop.forced_string: "true"
    shop.constant: !php/const Shop\Config::LIMIT
    shop.list:
        - one
        - two
    shop.map:
        a: 1
        b:
            - nested
`, output)
}

func TestConvertImports(t *testing.T) {
	output := convertString(t, `<container>
    <imports>
        <import resource="packages/listeners.xml"/>
        <import resource="other.yaml" ignore-errors="true"/>
        <import resource="not_found.yaml" ignore-errors="not_found"/>
    </imports>
</container>`)

	assert.Equal(t, `imports:
    - {resource: packages/listeners.xml}
    - {resource: other.yaml, ignore_errors: true}
    - {resource: not_found.yaml, ignore_errors: not_found}
`, output)
}

func TestConvertDefaults(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <defaults autowire="true" autoconfigure="true" public="false">
            <bind key="$projectDir">%kernel.project_dir%</bind>
            <bind key="$logger" type="service" id="logger"/>
        </defaults>
        <service id="Shop\A" class="Shop\A"/>
    </services>
</container>`)

	assert.Equal(t, `services:
    _defaults:
        autowire: true
        autoconfigure: true
        public: false
        bind:
            $projectDir: '%kernel.project_dir%'
            $logger: '@logger'

    Shop\A: ~
`, output)
}

func TestConvertPrototype(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <prototype namespace="Shop\" resource="../src/*" exclude="../src/{DependencyInjection,Entity,Kernel.php}"/>
        <prototype namespace="Shop\Handler\" resource="../src/Handler/*">
            <exclude>../src/Handler/Legacy/*</exclude>
            <exclude>../src/Handler/Experimental/*</exclude>
            <tag name="shop.handler"/>
        </prototype>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\:
        resource: ../src/*
        exclude: ../src/{DependencyInjection,Entity,Kernel.php}

    Shop\Handler\:
        resource: ../src/Handler/*
        exclude:
            - ../src/Handler/Legacy/*
            - ../src/Handler/Experimental/*
        tags:
            - shop.handler
`, output)
}

func TestConvertInstanceof(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <instanceof id="Shop\HandlerInterface" autowire="true" public="false">
            <tag name="shop.handler"/>
        </instanceof>
        <service id="Shop\A" class="Shop\A"/>
    </services>
</container>`)

	assert.Equal(t, `services:
    _instanceof:
        Shop\HandlerInterface:
            public: false
            autowire: true
            tags:
                - shop.handler

    Shop\A: ~
`, output)
}

func TestConvertAliases(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="shop.short_alias" alias="Shop\Service"/>
        <service id="shop.public_alias" alias="Shop\Service" public="true"/>
        <service id="shop.deprecated_alias" alias="Shop\Service">
            <deprecated package="shop/plugin" version="1.2">The "%alias_id%" alias is deprecated.</deprecated>
        </service>
    </services>
</container>`)

	assert.Equal(t, `services:
    shop.short_alias: '@Shop\Service'

    shop.public_alias:
        alias: Shop\Service
        public: true

    shop.deprecated_alias:
        alias: Shop\Service
        deprecated:
            package: shop/plugin
            version: "1.2"
            message: The "%alias_id%" alias is deprecated.
`, output)
}

func TestConvertDeprecatedService(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\Old" class="Shop\Old">
            <deprecated package="shop/plugin" version="2.0">Use "Shop\New" instead.</deprecated>
        </service>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\Old:
        deprecated:
            package: shop/plugin
            version: "2.0"
            message: Use "Shop\New" instead.
`, output)
}

func TestConvertFactories(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\FromClass" class="Shop\FromClass">
            <factory class="Shop\Factory" method="create"/>
        </service>
        <service id="Shop\FromService" class="Shop\FromService">
            <factory service="shop.factory" method="create"/>
        </service>
        <service id="Shop\FromInvokable" class="Shop\FromInvokable">
            <factory service="shop.factory"/>
        </service>
        <service id="Shop\FromFunction" class="Shop\FromFunction">
            <factory function="shop_create"/>
        </service>
        <service id="Shop\FromExpression" class="Shop\FromExpression">
            <factory expression="service('shop.factory').create()"/>
        </service>
        <service id="Shop\FromOwnMethod" class="Shop\FromOwnMethod">
            <factory method="create"/>
        </service>
        <service id="Shop\Configured" class="Shop\Configured">
            <configurator service="shop.configurator" method="configure"/>
        </service>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\FromClass:
        factory: [Shop\Factory, create]

    Shop\FromService:
        factory: ['@shop.factory', create]

    Shop\FromInvokable:
        factory: ['@shop.factory', __invoke]

    Shop\FromFunction:
        factory: shop_create

    Shop\FromExpression:
        factory: '@=service(''shop.factory'').create()'

    Shop\FromOwnMethod:
        factory: [null, create]

    Shop\Configured:
        configurator: ['@shop.configurator', configure]
`, output)
}

func TestConvertCalls(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\A" class="Shop\A">
            <call method="setLogger">
                <argument type="service" id="logger"/>
            </call>
            <call method="init"/>
            <call method="withPriority" returns-clone="true">
                <argument>5</argument>
            </call>
        </service>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\A:
        calls:
            - [setLogger, ['@logger']]
            - [init]
            - [withPriority, [5], true]
`, output)
}

func TestConvertTags(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\Listener" class="Shop\Listener">
            <tag name="kernel.event_listener" event="kernel.request" method="onRequest" priority="-10"/>
            <tag name="shop.custom" some-attribute="value"/>
            <tag>shop.text_tag</tag>
            <tag name="shop.nested">
                <attribute name="config">
                    <attribute name="enabled">true</attribute>
                </attribute>
            </tag>
        </service>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\Listener:
        tags:
            - {name: kernel.event_listener, event: kernel.request, method: onRequest, priority: -10}
            - {name: shop.custom, some_attribute: value, some-attribute: value}
            - shop.text_tag
            - {name: shop.nested, config: {enabled: true}}
`, output)
}

func TestConvertArgumentTypes(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\A" class="Shop\A">
            <argument type="service" id="required"/>
            <argument type="service" id="optional" on-invalid="ignore"/>
            <argument type="service" id="nullable" on-invalid="null"/>
            <argument type="service" id="uninitialized" on-invalid="ignore_uninitialized"/>
            <argument type="expression">service('shop').isActive()</argument>
            <argument type="string">@literal</argument>
            <argument type="constant">PHP_INT_MAX</argument>
            <argument type="tagged_iterator" tag="shop.handler"/>
            <argument type="tagged_iterator" tag="shop.handler" index-by="key" default-index-method="getKey" default-priority-method="getPriority"/>
            <argument type="tagged_locator" tag="shop.handler"/>
            <argument type="service_closure" id="closed"/>
            <argument type="iterator">
                <argument type="service" id="one"/>
                <argument type="service" id="two"/>
            </argument>
            <argument type="service_locator">
                <argument key="first" type="service" id="one"/>
                <argument key="second" type="service" id="two"/>
            </argument>
            <argument type="collection">
                <argument>plain</argument>
                <argument type="service" id="inside"/>
            </argument>
        </service>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\A:
        arguments:
            - '@required'
            - '@?optional'
            - '@?nullable'
            - '@!uninitialized'
            - '@=service(''shop'').isActive()'
            - '@@literal'
            - !php/const PHP_INT_MAX
            - !tagged_iterator shop.handler
            - !tagged_iterator {tag: shop.handler, index_by: key, default_index_method: getKey, default_priority_method: getPriority}
            - !tagged_locator shop.handler
            - !service_closure '@closed'
            - !iterator
              - '@one'
              - '@two'
            - !service_locator
              first: '@one'
              second: '@two'
            - - plain
              - '@inside'
`, output)
}

func TestConvertTaggedIteratorWithExcludes(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\A" class="Shop\A">
            <argument type="tagged_iterator" tag="shop.handler" exclude-self="false">
                <exclude>Shop\ExcludedHandler</exclude>
            </argument>
        </service>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\A:
        arguments:
            - !tagged_iterator {tag: shop.handler, exclude: [Shop\ExcludedHandler], exclude_self: false}
`, output)
}

func TestConvertNamedArguments(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\A" class="Shop\A">
            <argument key="$limit">10</argument>
            <argument key="$logger" type="service" id="logger"/>
        </service>
        <service id="Shop\Child" parent="Shop\A">
            <argument index="0">20</argument>
        </service>
        <service id="Shop\Mixed" class="Shop\Mixed">
            <argument>positional</argument>
            <argument key="$named">value</argument>
        </service>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\A:
        arguments:
            $limit: 10
            $logger: '@logger'

    Shop\Child:
        parent: Shop\A
        arguments:
            index_0: 20

    Shop\Mixed:
        arguments:
            0: positional
            $named: value
`, output)
}

func TestConvertProperties(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\A" class="Shop\A">
            <property name="logger" type="service" id="logger"/>
            <property name="limit">25</property>
        </service>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\A:
        properties:
            logger: '@logger'
            limit: 25
`, output)
}

func TestConvertServiceBind(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\A" class="Shop\A">
            <bind key="$shopName">Demo</bind>
        </service>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\A:
        bind:
            $shopName: Demo
`, output)
}

func TestConvertInlineService(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\A" class="Shop\A">
            <argument type="service">
                <service class="Shop\Inline">
                    <argument>setting</argument>
                </service>
            </argument>
        </service>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\A:
        arguments:
            - !service
              class: Shop\Inline
              arguments:
                - setting
`, output)
}

func TestConvertWhenEnv(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\A" class="Shop\A"/>
    </services>
    <when env="dev">
        <parameters>
            <parameter key="shop.debug">true</parameter>
        </parameters>
        <services>
            <service id="Shop\DevOnly" class="Shop\DevOnly"/>
        </services>
    </when>
</container>`)

	assert.Equal(t, `services:
    Shop\A: ~

when@dev:
    parameters:
        shop.debug: true
    services:
        Shop\DevOnly: ~
`, output)
}

func TestConvertFileElement(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\Legacy" class="Shop\Legacy">
            <file>%kernel.project_dir%/legacy/Legacy.php</file>
        </service>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\Legacy:
        file: '%kernel.project_dir%/legacy/Legacy.php'
`, output)
}

func TestConvertEmptyContainer(t *testing.T) {
	output := convertString(t, `<container/>`)
	assert.Equal(t, "", output)
}

func TestConvertKeepsDocumentOrder(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <prototype namespace="Shop\" resource="../src/*"/>
        <service id="Shop\Special" class="Shop\Special" public="true"/>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\:
        resource: ../src/*

    Shop\Special:
        public: true
`, output)
}

func TestConvertErrors(t *testing.T) {
	tests := []struct {
		name        string
		xml         string
		expectedErr string
	}{
		{
			name:        "unknown element in container",
			xml:         `<container><monolog/></container>`,
			expectedErr: "unsupported element <monolog> inside <container>",
		},
		{
			name:        "stack is not supported",
			xml:         `<container><services><stack id="foo"/></services></container>`,
			expectedErr: "unsupported element <stack> inside <services>",
		},
		{
			name:        "unknown attribute on service",
			xml:         `<container><services><service id="foo" class="Foo" scope="prototype"/></services></container>`,
			expectedErr: `unsupported attribute "scope" on <service>`,
		},
		{
			name:        "unknown element in service",
			xml:         `<container><services><service id="foo" class="Foo"><something/></service></services></container>`,
			expectedErr: "unsupported element <something> inside <service>",
		},
		{
			name:        "duplicate service id",
			xml:         `<container><services><service id="foo" class="Foo"/><service id="foo" class="Bar"/></services></container>`,
			expectedErr: `service "foo" is defined multiple times`,
		},
		{
			name:        "service without id",
			xml:         `<container><services><service class="Foo"/></services></container>`,
			expectedErr: "<service> requires an id attribute",
		},
		{
			name:        "prototype without resource",
			xml:         `<container><services><prototype namespace="Foo\"/></services></container>`,
			expectedErr: "requires a resource attribute",
		},
		{
			name:        "alias with class",
			xml:         `<container><services><service id="foo" alias="bar" class="Foo"/></services></container>`,
			expectedErr: "aliases only support the public attribute",
		},
		{
			name:        "service reference without id",
			xml:         `<container><services><service id="foo" class="Foo"><argument type="service"/></service></services></container>`,
			expectedErr: `argument type="service" requires an id attribute`,
		},
		{
			name:        "unsupported argument type",
			xml:         `<container><services><service id="foo" class="Foo"><argument type="wild"/></service></services></container>`,
			expectedErr: `unsupported argument type "wild"`,
		},
		{
			name:        "unsupported on-invalid",
			xml:         `<container><services><service id="foo" class="Foo"><argument type="service" id="bar" on-invalid="explode"/></service></services></container>`,
			expectedErr: `unsupported on-invalid value "explode"`,
		},
		{
			name:        "tag without name",
			xml:         `<container><services><service id="foo" class="Foo"><tag/></service></services></container>`,
			expectedErr: "<tag> requires a name",
		},
		{
			name:        "call without method",
			xml:         `<container><services><service id="foo" class="Foo"><call/></service></services></container>`,
			expectedErr: "<call> requires a method attribute",
		},
		{
			name:        "factory without target",
			xml:         `<container><services><service id="foo" class="Foo"><factory/></service></services></container>`,
			expectedErr: "<factory> requires a class, service, function or expression attribute",
		},
		{
			name:        "inline service in factory",
			xml:         `<container><services><service id="foo" class="Foo"><factory method="create"><service class="Bar"/></factory></service></services></container>`,
			expectedErr: "unsupported element <service> inside <factory>",
		},
		{
			name:        "bind without key",
			xml:         `<container><services><service id="foo" class="Foo"><bind>value</bind></service></services></container>`,
			expectedErr: "<bind> requires a key attribute",
		},
		{
			name:        "when without env",
			xml:         `<container><when><services/></when></container>`,
			expectedErr: "<when> requires an env attribute",
		},
		{
			name:        "multiple defaults",
			xml:         `<container><services><defaults autowire="true"/><defaults public="false"/></services></container>`,
			expectedErr: "multiple <defaults> elements are not supported",
		},
		{
			name:        "namespace on regular service",
			xml:         `<container><services><service id="foo" class="Foo" namespace="Bar\"/></services></container>`,
			expectedErr: "namespace, resource and exclude are only supported on <prototype>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := convertErr(t, tt.xml)
			assert.ErrorContains(t, err, tt.expectedErr)
		})
	}
}

func TestConvertInvalidXML(t *testing.T) {
	_, err := ParseServicesXML([]byte(`<routes/>`))
	assert.Error(t, err)

	_, err = ParseServicesXML([]byte(`not xml`))
	assert.Error(t, err)
}

func TestConvertMultilineArgument(t *testing.T) {
	output := convertString(t, `<container>
    <services>
        <service id="Shop\A" class="Shop\A">
            <argument>line1
line2</argument>
        </service>
        <service id="Shop\B" class="Shop\B"/>
    </services>
</container>`)

	assert.Equal(t, `services:
    Shop\A:
        arguments:
            - |-
              line1
              line2

    Shop\B: ~
`, output)
}
