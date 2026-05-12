package html

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Regression test for https://github.com/shopware/shopware-cli/issues/1002
// Nested {% if %} blocks within another {% if %} branch caused parseIfBranch
// to consume the inner {% endif %} as the outer one and then fail to find
// the real outer {% endif %}.
func TestParseNestedIfInIfBranch(t *testing.T) {
	tpl := `{% block buy_widget_price_unit_content_custom %}
    {% if not isFractional %}
        {% if customFields['schq_quantity_info_number_of_subitems'] == 1 %}
            1 {{ packUnit }}
        {% else %}
            {{ packUnit }}
            {% if isWeighted %}
                ca.
            {% else %}
                à
            {% endif %}
            {{ customFields['schq_quantity_info_number_of_subitems']|format_number }} {{ customFields['schq_quantity_info_packaging_unit'] }}
        {% endif %}
    {% else %}
        {{ 'fractionalOrderQuantity.fractionalQuantity'|trans }}: <br>
        {{ customFields['schq_quantity_info_fractional_selling_unit'] }}
        {{ customFields['schq_quantity_info_packaging_unit'] }}
    {% endif %}
{% endblock %}
`

	_, err := NewParser(tpl)
	assert.NoError(t, err)
}

func TestParseNestedIfInElseBranch(t *testing.T) {
	tpl := `{% if outer %}
    outer-true
{% else %}
    {% if inner %}
        inner-true
    {% else %}
        inner-false
    {% endif %}
{% endif %}
`

	_, err := NewParser(tpl)
	assert.NoError(t, err)
}

func TestParseTripleNestedIf(t *testing.T) {
	tpl := `{% if a %}
    {% if b %}
        {% if c %}
            deep
        {% endif %}
    {% endif %}
{% endif %}
`

	_, err := NewParser(tpl)
	assert.NoError(t, err)
}
